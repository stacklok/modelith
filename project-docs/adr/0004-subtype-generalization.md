# Subtype / generalization via `subtypeOf`

Add an is-a link between entities: `subtypeOf: <Parent>` as a top-level field on
the child entity. Resolves issue #8.

## Context

A relationship could say an entity *has* or *references* another, but not that
one entity *is a kind of* another. Generalization is bread-and-butter domain
modeling and models hit it early (for example a `Notification` that is exactly
one of `Email`, `Sms`, or `Push`). The is-a claim lived only in
definition prose, so the hierarchy was invisible to the renderer, parent
invariants did not formally bind to children, and the linter could not check
the enumeration.

## Decision

The child declares its own membership: `subtypeOf: Parent`.

## Considered options

- **`subtypes: [...]` on the parent** — rejected. It couples the parent to every
  child and does not scale to an open hierarchy. The child knowing its own
  supertype is the natural direction and mirrors how a reader states it ("a
  `Shape` is a `Node`").
- Exhaustiveness / disjointness markers (`sealed`, `abstract`) — deferred until
  a second model needs them.

## Consequences

- Lint: `subtypeOf` naming an undefined entity is an error, like a
  relationship target that names no entity.
- Completeness: a parent's invariants formally cover its subtypes, so a subtype
  with no invariants of its own no longer triggers the "has no invariants"
  warning when its parent has them. This is a real change to the completeness
  pass.
- Render (per ADR-0002): the is-a link is captured in the schema and shown as a
  Markdown hierarchy section; the ER does not fake it with a crow's-foot edge.
  A truthful visual (a supplementary class diagram, or a future renderer) is a
  later upgrade.

The enforcing tests (`TestADR_0004_*`) and full implementation land with #8.
