# Differences from upstream

This fork follows [Notifuse/liquidgo](https://github.com/Notifuse/liquidgo) and is merged from its
release tags. It differs from upstream in one place, listed here. Every other line of `liquid/` is
upstream's.

This file is the fork's own, so a merge from upstream never conflicts with it.

## An assign is not scored when no limit is set

Issue [#7](https://github.com/zeroedin/liquidgo/issues/7).

### The rule

1. **With no assign score limit set**, an `{% assign %}` adds nothing to the assign score, and the
   value it assigns is not walked to score it.
2. **With a limit set**, nothing changes. The value is scored as upstream scores it, and going over
   the limit ends the render with `Memory limits exceeded`. A limit can come from the environment
   (`EnvironmentConfig.Limits`) or from a render (`liquid.WithLimits`).
3. **"No limit" means `AssignScoreLimit` is `nil`.** A limit of `0` is a limit: any assign goes
   over it.
4. **Nothing else changes.** `capture` still adds what it captures to the assign score. The render
   score is still counted. The assigned value is the same.

So with no limit set, `Result.AssignScore` counts only what `capture` wrote.

### Why

Upstream scores every assign, as Shopify's Liquid does, so that a render can report its score. To
score a value it walks all of it: every item of a list, every key and value of a hash, at every
level.

A host that assigns a large collection in a layout pays for that walk on every page, and reads
nothing from the score when no limit is set. Measured in Alloy, on a 1,000-page site whose layout
has `{% assign posts = collections.blog %}`, re-rendering every page took:

| Library | Time |
|---|---|
| Upstream v5.14.5 | 0.59s |
| Upstream v5.14.5 with this rule | 0.18s |

### Tests

- `integration/assign_score_skip_test.go`: the rule above.
- Upstream tests changed for it:
  - `integration/assign_test.go`: `assignScoreOf` sets a limit that is never reached, so the four
    `TestAssignScoreOf*` tests still check how a value is scored.
  - `integration/template_test.go`: `TestResourceLimitsHashInTemplateGetsUpdatedEvenIfNoLimitsAreSet`
    and `TestDefaultResourceLimitsUnaffectedByRenderWithContext` expect an assign score of 0.
  - `integration/render_test.go`: `TestRenderOptions` sets a limit that is never reached where it
    checks that a render reports both scores.

When a merge from upstream changes one of these tests, keep the change above.
