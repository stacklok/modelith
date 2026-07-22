# Structured value types are out of scope

Do not add a `types:` section or any structured value-type construct. A value
type with named, typed fields defines record structure — implementation
semantics modelith deliberately does not capture. Supersedes ADR-0005.

## Context

ADR-0005 decided to add a top-level `types:` section: named value types with
`fields` (name + type) and prose `constraints`, resolvable from an attribute
`type`, with `repeated: true` for collections. Nothing was built. On revisiting
the shape before implementation, the construct read as too close to an
interface definition language: a list of named, typed fields is a record
declaration — "define the columns" — which is exactly the implementation
detail the format is meant to abstract away.

## Decision

Structured value types are out of scope. modelith captures the *general shape of
a system* — the concepts, their relationships, the rules that must hold — not
the internal field layout of a value. A value-shaped concept is modeled the way
the schema reference already prescribes: as an owned entity, or as attributes on
its owner, with any internal rules stated as prose (a glossary entry or an
invariant).

## Why

- **It crosses the IDL line.** The format's identity is a communication artifact,
  not a schema language. `{amount: decimal, currency: Currency}` is a data
  structure definition; that belongs in code or an IDL, not here (the same line
  ADR-0006 held for action reports).
- **The general shape is already expressible.** Owned entities and attributes
  cover value-shaped concepts well enough to communicate intent. Where identity
  is genuinely undeserved, the tension is named in prose (the parking-garage
  `Ticket`), which is honest about the trade-off rather than papering over it.
- **"For now," not "never."** If a real model demonstrates that prose plus an
  owned entity genuinely cannot carry a needed distinction, revisit — but the
  default stance is omission, and the bar to reopen is a concrete modeling
  failure, not expressiveness for its own sake.

## Consequences

- The schema reference keeps value objects in "what this format deliberately
  leaves out," reframed from "being explored" to a settled omission.
- Issue #11 is closed as out of scope rather than implemented.
- Issue #4 sub-item 4 (the `Ticket` value-object teaching moment) no longer
  waits on value types; the current owned-entity-with-named-tension treatment
  is the answer.
- Nothing to pin with a test: the decision is the absence of a construct, and
  the schema already has no `types:` section. The human-facing guard is the
  "deliberately leaves out" section of the schema reference.
