# Richer relationship cardinality: bounds and symmetry

Extend relationship `cardinality` to express bounds and exact counts, and add a
`symmetric` marker for unordered relationships. Resolves issues #9 and #10.

## Context

`cardinality` was an enum of `1:1`, `1:n`, `n:1`, `n:n`. Real models needed
exact and bounded counts — a relationship that must have exactly two members —
and genuinely symmetric relationships, where an unordered pair (peering,
adjacency, friendship) has no natural direction. Both were smeared across a
freeform `role` string and a prose invariant, none of it checkable, and the ER
drew "many" where the truth was "exactly two."

## Decision

**Bounds (#9).** `cardinality` becomes a pattern-validated string. Each side is
a multiplicity:

```
multiplicity ::= 1 | n | 0..1 | 1..n | <int> | <int>..<int> | <int>..n
```

The four original values keep their exact meaning as shorthand, so no existing
model changes. Examples: `1:2` (exactly two), `1:0..1` (optional), `1:1..n`
(at least one). `InvertCardinality` generalizes to swap the two sides
(`1:2` → `2:1`), which stays backward compatible for the shorthands.

**Symmetry (#10).** A `symmetric: true` flag on a relationship means it carries
no inherent order: `(a, b) == (b, a)`. Valid on a self-referential
relationship, or on one whose target multiplicity is greater than one. Composed
with bounds, `1:2` + `symmetric: true` declares an unordered pair.

## Consequences

- Render maps the target multiplicity to the nearest crow's-foot glyph
  (min 0 → `o`, min ≥1 → `|`; max 1 → `|`, max >1 → `{`). Exact counts render
  as one-or-many; the precise number lives in the Markdown line and role. This
  also fixes the audit foot-gun where `1:n` always drew zero-or-many even when
  a prose invariant said "at least one" — the modeler now writes `1:1..n`.
- Symmetric relationships stay one edge, not two, and the label carries
  "unordered"; per ADR-0002 the ER does not attempt an undirected glyph.
- Exact counts (`2`) are documentation, not structurally enforceable against
  instance data. That is accepted: the value is an honest role and a
  less-wrong glyph.

The enforcing tests (`TestADR_0003_*`) and full implementation land with the
feature work tracked in #9 and #10.
