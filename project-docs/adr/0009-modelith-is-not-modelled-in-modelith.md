# modelith is not modelled in modelith

Do not write a `*.modelith.yaml` describing modelith's own domain — the domain
model format itself. The vocabulary it would capture is already defined in
three places that tests hold in sync; a self-model would be a fourth copy that
nothing checks, and it would drift.

## Context

modelith's domain is the domain-model format: entities, attributes,
relationships, invariants, actions, scenarios, enums, glossary terms. Modelling
that in modelith is the obvious dogfooding move, and the question comes up
naturally — most recently while adapting a design-interview skill whose
upstream version maintains a `CONTEXT.md` glossary.

## Decision

There is no self-model, and no `CONTEXT.md` either. The format's vocabulary
lives where it already lives:

- `internal/schema/v1/modelith.schema.json` — the normative definition.
- `internal/model/model.go` — the Go structs, held property-for-property
  against the schema by `TestSchemaStructSync`.
- `docs/06-schema-reference.md` — the prose definition users read.
- `examples/example.modelith.yaml` and `docs/05-parking-garage/` — the worked
  examples, rendered under `task render-check` and pinned by a golden test.

## Why

- **A fourth copy that nothing checks.** Two of the three definitions are
  machine-checked against each other. A self-model would join them as prose
  nobody validates, and drift between a tool's own model and its own schema is
  the worst possible advertisement for the tool.
- **It would compete with the worked examples.** `examples/` exists to show what
  a good model looks like, and it is a golden fixture. A second in-repo model
  with a stronger claim to being "the real one" muddies which file a newcomer
  should read, and invites edits made for demo reasons.
- **The self-referential framing costs more than it teaches.** A model whose
  `Entity` entity has an `attributes` relationship to an `Attribute` entity
  reads as a puzzle. Readers learning the format are better served by the
  parking garage.
- **Dogfooding still happens, elsewhere.** The format is exercised by real
  models in other repos, which is where the pressure that produced issues #8–#13
  and #24–#25 came from. That is the useful kind of dogfooding: a model of a
  different domain, hitting real gaps.

## Consequences

- `.claude/skills/grill-with-docs/` states this so a design-interview session
  doesn't helpfully start one; it also drops the upstream skill's `CONTEXT.md`
  behavior for the same reason.
- Format vocabulary changes land as one change across the schema, the structs,
  and the schema reference — never as an edit to a self-model.
- Pinned by `TestADR_0009_NoSelfModel`, which asserts the set of committed
  `*.modelith.yaml` files is exactly the known examples. Adding a legitimate
  new example means extending that list, which puts this ADR in front of
  whoever does it.
- Reopen if the format grows a construct that only a self-model can exercise —
  a concrete gap, not the appeal of dogfooding.
