---
paths:
  - "**/*.go"
description: Go coding conventions for modelith
---

# Go style & conventions

Path-triggered companion to `CLAUDE.md`. This file loads on every Go-file
edit. Where `CLAUDE.md` and this file disagree on a Go-specific point, this
file wins.

## Module & build

- Module: `github.com/stacklok/modelith`.
- Go version: check `go.mod` for the exact version.
- Build via `task build`, not bare `go build`. The Taskfile is the one place
  build flags (`LDFLAGS`) live; routing through it keeps them centralized.
- The binary is static: `CGO_ENABLED=0`, no cgo dependencies. Verify a new
  dependency doesn't pull in cgo before adding it.
- One binary, no runtime dependencies. modelith reads and writes files and
  prints diagnostics. If a feature seems to need a network call, a daemon, or
  a browser, that's a sign to redesign it, not to add the dependency.

## Dependencies

- Keep the dependency set small; every addition is a supply-chain and
  binary-size cost. Prefer the standard library.
- The CLI is built on `cobra`. Keep command wiring in `cmd/modelith/`; keep
  the logic it calls in `internal/`.
- Schema and example bytes are embedded with `go:embed` (see
  `internal/schema/`). Never read them from a path at runtime; embedding is
  what lets the single binary validate without fetching the canonical URL.

## Linting

- CI runs `go vet`, `staticcheck` (pinned in the Taskfile), and
  `golangci-lint run`. Run `task check` before pushing; it runs all three.
- Fix the root cause of a finding. Don't suppress with `//nolint` except for
  a documented false positive. The suppression comment names the linter rule
  and states why the finding doesn't apply here.

## Errors

- Return errors. Don't swallow them silently.
- Check errors with `errors.Is()` and `errors.As()`, not string comparison.
- Wrap with `fmt.Errorf("doing X: %w", err)` so callers can unwrap the chain.
  The prefix is a noun phrase naming what the code was doing, not the function
  name. Write `"reading model file: %w"`, not `"ReadModel: %w"`.
- Handle an error or wrap and propagate it. Never do both. Printing an error
  and also returning it produces two reports of the same failure.
- Lint and validation diagnostics are the product, not errors to hide. A
  malformed model surfaces as structured findings the user can act on
  (element ID, file path, line or JSON pointer), not a bare Go error string.
  Keep large or user-authored content out of Go error messages: name the
  offending element and its location, don't embed the whole document.

## Panics

- Return errors. Don't panic. Panic only for a programmer bug the type system
  can't express, such as an unreachable `default:` in a switch meant to be
  exhaustive.
- `must*` helpers and hard test-failure assertions belong in `main()` and
  `_test.go` only. Library code always returns an error.
- `recover()` belongs only at the top of the command handler, the one place
  an unexpected panic must become a clean non-zero exit instead of a crash.
  Every other layer lets a panic propagate.

## CLI output

- Results the user asked for go to stdout. Diagnostics, progress, and errors
  go to stderr. A caller piping `modelith render` into a file must not get
  warnings mixed into the output.
- No third-party logging framework. This is a batch CLI, not a service; it
  stays silent on success and prints actionable diagnostics on failure.
- Exit codes carry meaning: zero on success, non-zero when lint finds errors
  or a command fails. Keep that contract stable; the GitHub Action and CI
  depend on it.

## Comments

Default to no comment. Add one only when removing it would confuse a future
reader: a hidden constraint, a subtle invariant, a workaround for a specific
bug, or behavior that would otherwise surprise.

Forbidden:

- Restating the code. `// increments the counter` above `count++` adds
  nothing.
- Restating the identifier name, such as `// version is the version`.
- Forward-looking status, such as "not yet implemented" or "for now." Docs and
  code read as always-true; history lives in commit messages,
  `project-docs/adr/`, and `project-docs/audits/`.
- A `_ = x // used later` placeholder. Delete the unused value or use it.

Encouraged:

- A citation on the line that encodes a guarded invariant. For example:
  `// TestSchemaStructSync guards this: every schema property has a matching
  struct field.`
- A subtle invariant the type system can't express.
- A non-obvious workaround, with the upstream reference that explains it.

Package docs are one line stating why the package exists. Exported identifiers
get a doc comment: one sentence stating the contract, not the implementation.

## Determinism

Rendering must be deterministic: the same `*.modelith.yaml` produces
byte-identical Markdown and Mermaid every run. This is enforced by the golden
example (`examples/example.modelith.md`) and its golden test.

- Go map iteration order is randomized per run. Any code that ranges over a
  map and emits output — rendering, serialization, an ordered diagnostic
  list — must sort the keys first.
- After changing the renderer or the example, run `task render` to regenerate
  the `.md`, then review the diff. `task render-check` and CI fail on drift.

## Regex: RE2 only

Go's `regexp` package uses RE2: no lookahead, no lookbehind, no
backreferences. Don't add a PCRE-style regex library to work around this.
RE2's guaranteed linear-time matching is the point. For validation too complex
for one pattern, use a broad regex match followed by plain Go code, rather than
one clever pattern.

## Wiring

- Keep `main()` and the cobra command functions thin: parse arguments, wire
  dependencies, delegate to `internal/` code.
- No `init()`-based registration of behavior, no package-level mutable
  singletons, no global state. A dependency a function needs comes in as a
  parameter or a constructor argument.

## Things that bite

- `go test ./...` doesn't enable the race detector by default. Add `-race` for
  any test that touches goroutines or shared state.

## See also

- [`testing.md`](testing.md) — testing discipline and the golden fixtures.
- [`adr.md`](adr.md) — ADR shape and template.
- [`../../docs/_docs-conventions.md`](../../docs/_docs-conventions.md) — prose
  and structure conventions for the published docs.
