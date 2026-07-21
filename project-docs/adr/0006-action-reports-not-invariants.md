# Action consequences are reports, not invariants

Add a minimal `reports:` list to the structured action form: prose strings
naming what an action tells the caller it did. No typed `inputs`/`reports`.
Resolves issue #13.

## Context

Actions were `{name, actor, preserves, description}` labels. A recurring need
is the "no surprises" contract: a delete that cascades to child records *and
the result lists everything it removed*; an operation that *reports each side
effect it triggered*. Each was written as a sentence inside an invariant
statement, fusing the rule with the report.

## Decision

Add `reports:` to the structured action: a list of plain prose strings, each a
consequence the action tells the caller about. Backtick entity names as in any
freeform text. No `inputs`, and reports carry no `type` field.

## Why reports belong on the action, not in an invariant

An **invariant is unconditional**: it must hold regardless of what any action
does. "There are no dangling `Endpoint`s" is an invariant. A statement of the
form **"when you do X, Y happens" is conditional** — it is a consequence of the
action, true only as a result of it, not a standing truth of the model.
Placing it in an invariant misfiles it: the invariant then only "holds after
delete." Its correct home is the action's own description, which is what
`reports:` provides.

## Considered options

- **Typed `inputs` + `reports`** (the issue's original) — rejected. Typed
  entries turn an action from a *description* into a *signature*, the clearest
  step toward an interface definition language. modelith is a communication
  artifact, not an IDL; rules and shapes stay prose (see ADR-0005).
- **Defer, keep prose in invariants** — rejected. The conditional-consequence
  argument above says the invariant is the wrong home even today; `reports:`
  is the small, honest fix.

## Consequences

`reports:` is prose, so the linter checks its presence and can resolve
backticked names for the existing unresolved-term warning, but does not type
its entries. Revisit typed entries only if a real need for machine-readable
action outputs appears across multiple models.

The enforcing tests (`TestADR_0006_*`) and full implementation land with #13.
