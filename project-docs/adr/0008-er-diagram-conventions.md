# ER diagram conventions: line style, labels, and self-relationships

Rendering a real 13-entity model made two defects obvious (issues
[#26](https://github.com/stacklok/modelith/issues/26),
[#27](https://github.com/stacklok/modelith/issues/27)): a self-referential
relationship drew a runaway arc that swamped the canvas, and long prose roles
collided with each other while `owned` relationships that also carried a role
lost their ownership signal entirely. Three conventions fix both. They refine
ADR-0002's "deliberately lossy view" rather than overturning it: nothing here
fakes structure the ER cannot honestly show.

## Decision

1. **`ownership` is the line style.** `owned` draws the identifying (solid)
   connector `--`; `referenced` and an omitted `ownership` draw the
   non-identifying (dashed) `..`. That is standard ER semantics for composition
   and costs no label space. Ownership is a property of the relationship, not of
   the end that declared it, so when a parent's `owned` and a child's
   `referenced` fold into one edge, the folded edge stays solid.
2. **`role` is the only label.** The `ownership` and `cardinality` fallbacks are
   gone; a relationship with no role gets an empty label. Precise counts already
   live in the Markdown table per ADR-0002, and spending label space on them
   crowds out the roles. A semantic lint *warning* (never an error) fires when a
   role reads as prose — more than four words, or sentence punctuation — and
   names `note` as the field for the explanation.
3. **A self-referential relationship is a row inside the entity's block**, not
   an edge: `Project self "0..1 — Predecessor"`. The row carries the target-side
   cardinality, the word `owned` when owned (there is no line to carry it), and
   the role. Rows are named `self`, `self2`, … because Mermaid does not
   disambiguate two attributes sharing a name.

## Evidence

Verified against `@mermaid-js/mermaid-cli@11.16.0` and GitHub's renderer, July
2026. Mermaid's dagre ER layout has no self-loop handling: `Record ||--o| Record`
draws an arc that dominates or overflows the canvas, independent of the label
text, so no label-side workaround exists. `layout: elk` fixes the arc but is
rejected — GitHub does not bundle `@mermaid-js/layout-elk`. Mermaid accepts a
PascalCase entity name as an attribute type, so the in-box row is valid source
for both renderers.

## Consequences

Point 3 is the one place a *relationship* is drawn inside a box, so the
"attributes are intentionally omitted from the diagram" rule from ADR-0002 now
reads: ordinary attributes are omitted, self-relationships are not. Ownership
becomes visible at a glance across the whole diagram, which makes an incorrect
`ownership` easier to spot than the old word label did.

Pinned by `TestADR_0008_*` in `internal/render/mermaid` and `TestProseRoleIsWarning`
in `internal/lint`.
