package integration

// The fork's one difference from upstream (issue #7, docs/differences-from-upstream.md):
// with no assign score limit set, an assign tag does not score the value it
// assigns. Scoring walks the whole value, which costs a host that assigns
// large collections on every render, and nothing reads the score when no
// limit is set. With a limit set, nothing changes.

import (
	gocontext "context"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/liquidgo/v5/liquid"
	"github.com/Notifuse/liquidgo/v5/liquid/tags"
)

// skipRender renders src with data in env and fails the test on a Go error.
func skipRender(t *testing.T, env *liquid.Environment, src string, data map[string]interface{}, opts ...liquid.RenderOption) liquid.Result {
	t.Helper()
	tpl, err := liquid.ParseTemplate(src, &liquid.TemplateOptions{Environment: env})
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	res, err := tpl.Render(gocontext.Background(), data, opts...)
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return res
}

func skipLimit(n int) liquid.ResourceLimitsConfig {
	return liquid.ResourceLimitsConfig{AssignScoreLimit: &n}
}

// skipList returns a list of n ints.
func skipList(n int) []interface{} {
	list := make([]interface{}, n)
	for i := range list {
		list[i] = i
	}
	return list
}

func TestAssignWithNoLimitIsNotScored(t *testing.T) {
	env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})
	values := map[string]interface{}{
		"an int":   123,
		"a string": "abcd",
		"a list":   []interface{}{123, "abcd"},
		"a hash":   map[string]interface{}{"int": 123, "str": "abcd"},
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			res := skipRender(t, env, `{% assign x = value %}`, map[string]interface{}{"value": value})
			if res.AssignScore != 0 {
				t.Errorf("assign score %d with no limit set, want 0", res.AssignScore)
			}
		})
	}
}

// Assigning a large list costs about as much as assigning a small one when
// no limit is set: the list is not walked.
func TestAssignWithNoLimitDoesNotWalkTheValue(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})
	tpl, err := liquid.ParseTemplate(`{% for i in (1..50) %}{% assign x = value %}{% endfor %}`, &liquid.TemplateOptions{Environment: env})
	if err != nil {
		t.Fatal(err)
	}
	measure := func(value []interface{}) time.Duration {
		best := time.Duration(0)
		for range 5 {
			start := time.Now()
			if _, err := tpl.Render(gocontext.Background(), map[string]interface{}{"value": value}); err != nil {
				t.Fatal(err)
			}
			if d := time.Since(start); best == 0 || d < best {
				best = d
			}
		}
		return best
	}
	small, large := skipList(1), skipList(200_000)
	measure(small) // warm up
	// A loaded machine can slow one measurement: measure again before failing.
	var base, got time.Duration
	for round := 0; round < 3; round++ {
		base, got = measure(small), measure(large)
		if got < 4*base || got < 2*time.Millisecond {
			return
		}
	}
	t.Errorf("50 assigns of a 200,000-item list took %v, of a 1-item list %v: the list is walked", got, base)
}

func TestAssignWithLimitIsScored(t *testing.T) {
	const src = `{% assign a = 123 %}{% assign b = "abcd" %}{% assign c = list %}`
	data := map[string]interface{}{"list": []interface{}{123, "abcd"}}
	// 1 for the int, 4 for the string, 1 + 1 + 4 for the list.
	const want = 11

	t.Run("limit from the environment", func(t *testing.T) {
		env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard(), Limits: skipLimit(1000)})
		if res := skipRender(t, env, src, data); res.AssignScore != want {
			t.Errorf("assign score %d, want %d", res.AssignScore, want)
		}
	})
	t.Run("limit from the render", func(t *testing.T) {
		env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})
		if res := skipRender(t, env, src, data, liquid.WithLimits(skipLimit(1000))); res.AssignScore != want {
			t.Errorf("assign score %d, want %d", res.AssignScore, want)
		}
	})
}

func TestAssignOverTheLimitIsStillStopped(t *testing.T) {
	const src = `[{% assign big = list %}]`
	data := map[string]interface{}{"list": skipList(100)}
	// check renders src and wants the render ended by the memory limit.
	check := func(t *testing.T, env *liquid.Environment, src string, data map[string]interface{}, opts ...liquid.RenderOption) {
		t.Helper()
		tpl, err := liquid.ParseTemplate(src, &liquid.TemplateOptions{Environment: env})
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		res, err := tpl.Render(gocontext.Background(), data, opts...)
		if err == nil || !strings.Contains(err.Error(), "Memory limits exceeded") {
			t.Errorf("output %q, error %v, want the memory limit error", res.Output, err)
		}
	}

	t.Run("limit from the environment", func(t *testing.T) {
		env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard(), Limits: skipLimit(10)})
		check(t, env, src, data)
	})
	t.Run("limit from the render", func(t *testing.T) {
		env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})
		check(t, env, src, data, liquid.WithLimits(skipLimit(10)))
	})
	// A limit of 0 is a limit: only nil means no limit.
	t.Run("limit of 0", func(t *testing.T) {
		env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})
		check(t, env, `[{% assign x = 1 %}]`, nil, liquid.WithLimits(skipLimit(0)))
	})
}

func TestAssignWithNoLimitStillWorks(t *testing.T) {
	env := mustEnv(liquid.EnvironmentConfig{Tags: tags.Standard()})

	t.Run("the value is assigned", func(t *testing.T) {
		data := map[string]interface{}{"list": []interface{}{"a", "b", "c"}}
		res := skipRender(t, env, `{% assign x = list %}{{ x.size }} {{ x | join: "," }}`, data)
		if res.Output != "3 a,b,c" {
			t.Errorf("output %q, want %q", res.Output, "3 a,b,c")
		}
	})
	t.Run("capture is still scored", func(t *testing.T) {
		res := skipRender(t, env, `{% capture foo %}すごい{% endcapture %}`, nil)
		if res.AssignScore != 9 {
			t.Errorf("assign score %d after capturing 9 bytes, want 9", res.AssignScore)
		}
	})
	t.Run("the render score is still counted", func(t *testing.T) {
		res := skipRender(t, env, `{% for i in (1..10) %}{% assign x = i %}{% endfor %}`, nil)
		if res.RenderScore == 0 {
			t.Error("render score 0, want it counted")
		}
	})
}
