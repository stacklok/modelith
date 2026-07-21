# Capture structure the Mermaid ER cannot render

The domain model captures structure at full fidelity even when the Mermaid
`erDiagram` cannot draw it. Render fidelity is a separate, evolving concern, not
a constraint on what the format may express.

## Context

The schema-expressiveness work (issues #8–#13) surfaced hard Mermaid limits,
verified July 2026:

- No generalization / is-a notation
  ([mermaid-js/mermaid#4139](https://github.com/mermaid-js/mermaid/issues/4139),
  open since 2023; inheritance exists only in Mermaid *class* diagrams).
- Cardinality is crow's-foot only — `||` exactly-one, `o|` zero-or-one,
  `|{` one-or-many, `o{` zero-or-many. No numeric bound, so "exactly two" is
  not drawable.
- Per-entity styling (a dashed border for a derived entity) is unconfirmed for
  the Mermaid versions the docs site and GitHub markdown run.

## Decision

The model, its generated Markdown, and the linter carry the full truth. The
Mermaid ER is a deliberately lossy view: it shows the coarse relational
skeleton with the best available crow's-foot glyph, and never fakes structure
it cannot honestly show (no crow's-foot glyph standing in for an is-a edge, no
invented cardinality). Precise facts — exact counts, symmetry, the is-a
hierarchy, derived-ness, value-type structure — live in the Markdown text and
tables. This extends the existing precedent that attributes are omitted from
the ER and shown in the Markdown table instead.

## Consequences

A truthful *visual* for these constructs is a future upgrade, not dropped work:
a supplementary Mermaid class diagram for hierarchies, or eventually a
higher-fidelity (more expensive) renderer.

Every feature ADR that follows (0003–0006) is designed against this principle:
capture now, render as the tooling allows.
