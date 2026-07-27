---
name: domain-model-lint
description: >
  Run modelith lint on a domain model and explain the findings — a read-only
  review pass. Use when the user asks to check, validate, or review a
  *.modelith.yaml file, or wants to know what is missing or inconsistent. Turns
  the linter output into a prioritized, actionable summary. To actually change
  the model (add or edit entities, fix the gaps), use domain-model-author.
---

# Linting a domain model

Run the linter and interpret it for the user. The point is not a clean bill of
health — it's making gaps *visible* so the user can decide whether they matter.

## Run it

First confirm `modelith` is installed (`modelith --version`); if it is not on
`PATH`, tell the user to install it before going further — don't eyeball the
YAML in its place:

```sh
go install github.com/stacklok/modelith/cmd/modelith@latest
# or download a binary from https://github.com/stacklok/modelith/releases
```

```sh
modelith lint <file>...            # human-readable
modelith lint --format json <file> # when you need to parse findings
```

Use `--completeness error` only if the user wants gaps to be treated as hard
failures (e.g. a strict CI gate). It promotes the **completeness** warnings (the
advisory gaps listed below) to blocking errors. It does **not** affect semantic
errors — those (including a dangling `invariants_touched` id) block regardless of
the flag.

## Interpret the three layers

- **Structural (error).** The file violates the JSON Schema — wrong type,
  missing required field, bad `cardinality` — or names a cross-model reference
  (`scope.Name`) in `relationship.entity` or `subtypeOf`, which is not
  supported anywhere but an attribute `type`. Must be fixed; the model won't
  parse cleanly otherwise.
- **Semantic (error or warning).** Errors (always block, flag-independent): a
  relationship points at an entity that doesn't exist; a scenario's
  `invariants_touched` or an action's `preserves` names an invariant id no entity
  or top-level `invariants` entry declares; the same invariant id is declared
  twice (entity-level and top-level invariants share one id namespace); reciprocal cardinalities
  that aren't inverses; reciprocal declarations that both claim
  `ownership: owned`; an `imports` entry that's absolute, unreadable, not a
  domain model, binds a scope another entry already bound, resolves outside
  the repository holding this model, or is a bare path whose filename yields
  no valid scope slug; an attribute `type` containing a dot that isn't a
  well-formed `scope.Name` (a dot is always reserved for a cross-model
  reference — `decimal(10.2)` and `google.protobuf.Timestamp` are errors, not
  primitives); a `scope.Name` type whose scope no import binds, or that names
  no enum in the model it resolves to. Warnings: a backticked term resolves to
  no entity, role, or actor; an action `actor` that's neither an entity nor a
  glossary term; a PascalCase attribute `type` that names no defined enum —
  usually a typo or a concept that was never named; a relationship `role`
  written as prose, which belongs in `note`; an ambiguous reciprocal pairing,
  where one end declares the same relationship twice and the other declares it
  back. Decide which it is and propose the fix.
- **Completeness (advisory warning).** Gaps, not bugs: an entity with no
  invariants, an entity no scenario exercises, a glossary term defined but
  never referenced, an enum no attribute uses, or an import nothing
  references. `shared: true` on the model being linted relaxes the two
  vocabulary-only checks (the unused glossary term and unused enum) — it does
  not relax the invariant or scenario-coverage checks, and it does nothing for
  an unreferenced import (that check is about *this* model's own `imports`,
  not about being imported). These are what `--completeness error` promotes to
  blocking. For each, ask whether it's a real gap (write the missing invariant
  or scenario, or drop the unused import) or genuinely fine — a
  `shared: true` model in particular.

## How to report back

1. Lead with **errors** — these block. Give the fix for each.
2. Then **semantic warnings** — likely typos or missing definitions.
3. Then **completeness gaps**, framed as questions: "Nothing exercises `Policy`
   in a scenario — should there be one, or is it intentionally a supporting
   concept?"

Don't just dump the linter output. Prioritize, explain *why* each matters, and
propose concrete edits. If the user agrees, use the `domain-model-author` skill
to make them, then re-lint and re-render.
