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
   renderer merges a pair of declarations in exactly two cases: a genuine
   reciprocal — the same pair, inverse cardinality, declared from *opposite*
   ends, with at most one end claiming `owned`, **and each end declaring that
   line only once** — and an exact duplicate from the same end. A genuine
   reciprocal draws solid if either end said `owned`, because ownership belongs
   to the relationship rather than to the end that named it.

   **Roles are not part of the predicate.** The two ends of a composition
   naturally name different roles — a parent's `part` is a child's `whole` —
   and that is the pattern the fold exists for, not a conflict. The single line
   is labelled by the owning end's role; with neither end owning, by the role
   from the end whose entity sorts first, so the choice never depends on
   declaration order.

   **The once-per-end condition is what makes the fold safe.** If one end
   declares the same line twice and the other declares it once, that one
   declaration is the reciprocal of one of the two — and the format cannot say
   which. Folding it into either drops the other's role, and which one depends
   on the order the declarations happen to be written in. So nothing folds for
   that pair: every declaration draws its own line. A lint **warning** names the
   pair, since more lines appear than the author probably means.

   Mutual `owned` is the one contradiction a reciprocal pair can hold — a
   relationship is owned by at most one end — and it is a lint **error**. The
   two lines both draw.

   **The guarantee, stated exactly.** Every declaration draws a line, except:
   one indistinguishable from an earlier declaration by the same entity (same
   line, same ownership, same role), which carries nothing the first does not;
   and the non-labelling end of a folded reciprocal, whose *role* is dropped
   from the diagram — the entity's own relationship list in the Markdown still
   carries it. That is ADR-0002's declaredly lossy view. Nothing else is
   dropped, and no output depends on declaration order beyond the order the
   lines are listed in.
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

Point 2 draws two lines where it cannot prove there is one. That is deliberate:
a diagram that quietly picks a winner among declarations hides a modeling
question only the author can settle. The one place it does pick — which role
labels a folded reciprocal — is a label choice inside a single line the model
does say is one relationship, and the dropped role is still in the Markdown.

The cost is that a pair with an ambiguous pairing renders more lines than its
author probably wants, with no way to collapse them short of editing the model.
That is the right side to err on: the alternative, a fold that guesses, is a
diagram whose content changes when the YAML is reordered. `model.EdgeGroups`
holds the one definition of which declarations could be the same line, and both
the renderer and the linter read it, so the fold rule and the diagnostic cannot
drift apart.

Pinned by `TestADR_0008_*` in `internal/render/mermaid` and in `internal/lint`.
