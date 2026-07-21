---
paths:
  - "**/*_test.go"
  - "project-docs/adr/**"
description: modelith testing discipline — naming, the guarded invariants, goldens
---

# Testing rules

## Test naming

A test that defends a rule recorded elsewhere carries that rule's identifier
in its name:

- `TestADR_NNNN_<ShortName>` for a rule codified in an ADR. Example:
  `TestADR_0002_SchemaURLStable`.
- `TestInvariant_<ShortName>` for a named invariant that has no ADR — a rule
  stated in `CLAUDE.md`, the schema, or the domain model. The existing
  `TestSchemaStructSync` and `TestURLConsistency` are exactly these; new
  invariant guards follow the prefix.

Everything else uses `Test<Subject>_<Behavior>`. A flat `go test -list` should
read as a coverage map: the `TestADR_` and `TestInvariant_` prefixes tell a
reader which tests pin a documented rule and which are ordinary unit tests.

A new ADR or invariant lands with its enforcing test in the same change. If
the rule can't be pinned by a test yet, say so in the ADR.

## The guarded invariants

modelith's correctness rests on a few cross-file invariants that fail quietly
if unguarded. Each already has, or must get, a named test:

- **Schema ↔ structs stay in sync.** `internal/schema/v1/modelith.schema.json`
  and `internal/model/model.go` agree property-for-property, every object
  `additionalProperties: false`. Guarded by `TestSchemaStructSync`.
- **The canonical schema URL is consistent** across the schema `$id`, the Go
  `URLFor`/`URL`, and the example header. Guarded by `TestURLConsistency`.
- **The example is a golden fixture.** `examples/example.modelith.yaml`
  renders to exactly `examples/example.modelith.md`, and lints clean under
  strict completeness. Guarded by the `internal/render/markdown` golden test
  and by `task lint-models` / `render-check` in CI.

Touching any of these surfaces means running `task check` locally before
pushing, not just the package's own tests.

## Determinism and goldens

Rendering is deterministic: the same YAML produces byte-identical Markdown and
Mermaid. Golden files are the right tool for that invariant, not an exception
to avoid.

- After changing the renderer or the example, run `task render` to regenerate
  `examples/example.modelith.md` (and the `docs/05-parking-garage/` example),
  then review the diff. Never regenerate and accept a golden without reading
  the diff: a golden change is a rendering-behavior change.
- Update goldens deliberately, as their own reviewed step, never as a side
  effect of making a test pass.
- Code that ranges over a map and emits output must sort keys first. Map
  iteration order is randomized per run; unsorted output is the usual source
  of a flaky golden.

## Fakes over mocks

Prefer a small hand-written fake over a generated mock for any interface
modelith defines, such as a file-system seam. A fake behaves like the real
dependency; a mock returns whatever the test told it to, whether or not the
real implementation could produce that response. Reserve mocking libraries for
narrow third-party interfaces modelith doesn't own and can't fake cheaply.

## Unit-test mechanics

- Table-driven by default. A handful of copy-pasted `t.Run` blocks with one
  changed value is a sign the test should be a table.
- `t.Parallel()` by default on top-level tests. Subtests that share fixture
  state stay sequential.
- The lint packages are the richest test target: cover structural, semantic,
  and completeness findings with concrete malformed fixtures, and assert the
  finding's kind and location, not just that some error occurred.

## Assertion quality

Default to exact equality, not existence checks (`NotEmpty`, `NotNil`). A
weaker assertion needs a documented reason.

- For lint output, assert the specific diagnostics produced, their severity,
  and where they point. A test that only checks "at least one error" passes
  even when the wrong rule fired.
- After a render, assert the actual bytes (via the golden), not merely that
  the output is non-empty.
- For a boundary rule (a completeness gap that is an error in strict mode but
  a warning otherwise), assert which case crosses the boundary, not just that
  some case eventually does.

## What we don't test

modelith doesn't test its JSON Schema library's or YAML library's internal
correctness. It tests its own interaction with them: that a given model, run
through modelith's schema dispatch and renderer, produces the diagnostics and
bytes modelith claims it will.

## Common rules

- When a test fails, fix the implementation, not the test.
- Run tests through `task test` (or `task check` for full CI parity), not a
  bare `go test` invocation, so the same flags and fixtures apply.
- Every new ADR and invariant lands with its enforcing test in the same change.

## See also

- [`adr.md`](adr.md) — ADR shape and template.
- [`go-style.md`](go-style.md) — Go coding conventions.
