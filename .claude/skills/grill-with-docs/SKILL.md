---
name: grill-with-docs
description: A relentless interview to sharpen a plan or design, capturing decisions in this repo's canonical homes as they land. Use when the user wants to stress-test a design and record what comes out of it.
disable-model-invocation: true
---

Run a `/grilling` session over the plan or decision at hand, then capture what
crystallises where it belongs. Capture inline, as each decision lands — not
batched at the end.

## Where things land in this repo

**Hard-to-reverse decisions with a real trade-off → write the ADR now.**
`project-docs/adr/`, shape and bar in [`.claude/rules/adr.md`](../../rules/adr.md).
Read that rule before writing one; it is stricter than the generic advice
(sequential numbering, supersede rather than edit, and a `TestADR_NNNN_*` test
when the decision is one a test can pin).

**Format or vocabulary changes → flag them, do not write them.** A new schema
key, a renamed concept, a changed definition: note it for the implementing
change and move on. Format vocabulary lives in three places that are checked
against each other — `internal/schema/v1/modelith.schema.json`,
`internal/model/model.go` (guarded by `TestSchemaStructSync`), and
`docs/06-schema-reference.md` — plus the golden `examples/`. Editing one of
them mid-grilling leaves the repo failing `task check` with the design still
half-argued.

**Running state → `HANDOFF.md`** at the end of the session (gitignored,
local-only; see `CLAUDE.md`).

## This repo has no CONTEXT.md, deliberately

The upstream version of this skill maintains a `CONTEXT.md` glossary. Don't
create one here. modelith's domain *is* the domain-model format, and that
vocabulary is already defined in the schema, the Go structs, and the schema
reference — two of which a test holds in sync. A `CONTEXT.md` would be a fourth
copy that nothing checks.

For the same reason, don't propose modelling modelith in `*.modelith.yaml`.
It would drift from the enforced definitions and compete with `examples/` for
the worked-example job.

## During the session

Beyond the interview itself, the angles worth pressing:

- **Sharpen fuzzy language.** When a term is vague or overloaded, propose a
  precise canonical one. Two names for one concept, or one name for two, is the
  bug that this whole tool exists to catch — don't let it pass in our own
  design conversation.
- **Stress-test with concrete scenarios.** Invent cases that probe the edges
  and force precision about where one concept stops and the next starts.
- **Cross-reference against the code.** When a claim is made about how modelith
  behaves, check it. The linter, the renderers, and the schema are right here;
  a contradiction between what we believe and what the binary does is a finding.
- **Probe emission frequency.** When the design emits something observable — a
  lint diagnostic, a log line, a rendered element — pin down how often it fires:
  once per model, per entity, or per attribute? A diagnostic that turns out to
  fire per attribute is a wall of output, and usually belongs on a different
  surface.
