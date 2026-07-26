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
   and costs no label space.
2. **Two declarations become one line only when they are one relationship.** The
   renderer merges a pair of declarations in exactly two cases: an exact
   duplicate from the same end, and a genuine reciprocal — the same pair,
   inverse cardinality, the same label, declared from *opposite* ends, with at
   most one end claiming `owned`. A genuine reciprocal draws solid if either end
   said `owned`, because ownership belongs to the relationship rather than to
   the end that named it. Every other case draws both lines, so no declaration
   disappears from the diagram: two declarations from the same end that differ
   only in `ownership` are two relationships, and mutual `owned` is a
   contradiction, not a fold.

   The contradictions the renderer refuses to swallow are lint **errors**:
   mutual `owned` from both ends, and a reciprocal pair that disagrees on
   ownership without folding (their roles differ, so the diagram would draw one
   solid and one dashed line for the same relationship).
3. **`role` is the only label.** The `ownership` and `cardinality` fallbacks are
   gone; a relationship with no role gets an empty label. Precise counts already
   live in the Markdown table per ADR-0002, and spending label space on them
   crowds out the roles. A semantic lint *warning* (never an error) fires when a
   role reads as prose — too long for a label, more than four words, or a
   sentence terminator — and names `note` as the field for the explanation.
4. **A self-referential relationship is a row inside the entity's block**, not
   an edge: `Project self "1:0..1 — Predecessor"`. The row carries the declared
   cardinality (both sides — the edge it replaces encoded both in its two end
   markers), the word `owned` when owned (there is no line to carry it), and the
   role. Rows are named `self`, `self2`, … because Mermaid does not
   disambiguate two attributes sharing a name; declarations that would render an
   identical row are emitted once.

## Evidence

Verified against `@mermaid-js/mermaid-cli@11.16.0` and GitHub's renderer, July
2026. Mermaid's dagre ER layout has no self-loop handling: `Record ||--o| Record`
draws an arc that dominates or overflows the canvas, independent of the label
text, so no label-side workaround exists. `layout: elk` fixes the arc but is
rejected — GitHub does not bundle `@mermaid-js/layout-elk`. Mermaid accepts a
PascalCase entity name as an attribute type, so the in-box row is valid source
for both renderers.

## Consequences

Point 4 is the one place a *relationship* is drawn inside a box, so the
"attributes are intentionally omitted from the diagram" rule from ADR-0002 now
reads: ordinary attributes are omitted, self-relationships are not. Ownership
becomes visible at a glance across the whole diagram, which makes an incorrect
`ownership` easier to spot than the old word label did.

Point 2 trades a tidier diagram for an honest one. A model whose two ends
disagree gets two lines and an error rather than a single line that quietly
picks a winner — the disagreement is a modeling question only the author can
settle.

Pinned by `TestADR_0008_*` in `internal/render/mermaid` and in `internal/lint`.
