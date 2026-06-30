# Working on modelith

Context for agents (and humans) working **on** modelith itself. For *using* the
tool, start with the [README](./README.md) and [`docs/`](./docs/).

modelith is a single static Go binary (cobra CLI: `lint`, `render`, `schema`)
that validates and renders `*.modelith.yaml` domain models. Module path
`github.com/stacklok/modelith`.

## Repository layout

```
modelith/
├── cmd/modelith/                 # CLI entrypoint (cobra)
├── internal/
│   ├── model/                    # Go structs + YAML (un)marshalling
│   ├── lint/                     # structural (schema) + semantic + completeness
│   ├── render/
│   │   ├── markdown/             # YAML → Markdown (embeds the Mermaid)
│   │   └── mermaid/              # YAML → Mermaid (erDiagram)
│   └── schema/                   # version registry + compile/dispatch
│       ├── schema.go
│       └── v1/modelith.schema.json   # canonical v1 schema (to be served at modelith.sh)
├── examples/                     # worked example: *.modelith.yaml + committed *.md (golden)
├── docs/                         # Docusaurus-importable docs
├── plugin/                       # Claude Code plugin (skills/)
├── audits/                       # dated design-decision records
├── action.yml · Taskfile.yml · .goreleaser.yaml · .github/workflows/
```

### Why this layout

- **`internal/schema/` is the source of truth for the format.** The Go code
  embeds each version's JSON Schema via `go:embed`; the canonical copy is
  destined for a stable URL (`https://modelith.sh/schema/domain-model/v1.json`)
  that editors will fetch via the `# yaml-language-server: $schema=` header once
  it's served (a roadmap item — not live yet; the CLI/CI embed the schema, so
  they don't depend on it). Living under `internal/` keeps the *Go API* private
  (`internal/` is a Go-compiler rule) without affecting URL reachability.
  Versions are directories (`v1/`,
  `v2/`, …) so the repo layout mirrors the URL layout and adding one is additive.
- **`internal/` not `pkg/`** — this is a tool, not a library. The CLI and the
  published schema are the contract; the Go API is private. Promote to a public
  API later only if there's demand.
- **`docs/` is self-contained Docusaurus content** built and served by the
  `website/` directory at [modelith.sh](https://modelith.sh).
- **`plugin/` ships the agent tooling next to the binary it drives**, so skills
  and the CLI version stay in lockstep.

## Dev workflow

The repo uses [Task](https://taskfile.dev). The one that matters:

```sh
task check   # run before pushing — CI parity plus local plugin validation
```

`task check` runs `vet`, `staticcheck`, `test`, `lint-models`, `render-check`,
and (locally only) `validate-plugin`. Run `task` with no arguments to list every
target. The full target table, build/install steps, and how to develop the
Claude Code plugin with `--plugin-dir` live in
[`docs/09-local-development.md`](./docs/09-local-development.md).

## Conventions to keep (these break CI if ignored)

- **The example is a golden fixture.** `examples/example.modelith.yaml` and its
  committed `examples/example.modelith.md` must stay in sync. After any change to
  the renderer *or* the example, run `task render` (or `modelith render
  examples/example.modelith.yaml`) to regenerate the `.md`. `task render-check` /
  CI fails on drift; `internal/render/markdown` has a golden test against it. The
  example must also lint clean under `task lint-models` (strict: completeness
  gaps are errors).
- **Schema ↔ structs stay in sync.** `internal/schema/v1/modelith.schema.json`
  and `internal/model/model.go` are guarded by `TestSchemaStructSync` (every
  schema property has a matching struct json field and vice versa). Every object
  is `additionalProperties: false`.
- **The canonical schema URL appears in three places** — the schema's `$id`, the
  Go `URLFor`/`URL` in `internal/schema/schema.go`, and the example header — and
  `TestURLConsistency` fails if they drift. Don't hardcode the URL elsewhere.
- **The binary, not the schema, owns supported versions.** `internal/schema`
  holds a `registry` (version → embedded bytes); `lint` reads the declared
  `version` and gives a friendly error before schema validation. Adding a format
  version = new `vN/` schema + a registry entry; never mutate a shipped version.
- **The `docs/` follow publishing conventions.** They are built by `website/`
  and served at [modelith.sh](https://modelith.sh). Page files are numbered
  `NN-name.md` (landing is plain `index.md`), carry `title:`, and cross-link with
  relative, prefix-included paths. The `docs/05-parking-garage/` example is
  lint/render-checked by CI, globbed **by path** in `Taskfile.yml` and
  `.github/workflows/ci.yml` — renumber that dir and you must update both. Full
  rules: [`docs/_docs-conventions.md`](./docs/_docs-conventions.md).

## Format decisions (already made — don't relitigate without reason)

- **Format evolution requires the new structured forms; no legacy string forms.**
  Invariants are `{id, statement}` referenced by `id`; enums are first-class
  (top-level `enums`, referenced from an attribute `type`); a top-level
  `glossary` defines non-entity vocabulary; actions are a bare string *or*
  `{name, actor?, preserves?, description?}`; attributes can be `derived` (with a
  required `derivation`). See [`docs/06-schema-reference.md`](./docs/06-schema-reference.md).
- **Stay on schema `v1`** while pre-release — there's no external `*.modelith.yaml`
  corpus to preserve, so the format evolves in place rather than bumping to v2.

## Cutting a release

Push a `vX.Y.Z` tag on `main` — `release.yml` builds, signs, generates SBOMs,
publishes the GitHub Release, and pushes the Homebrew formula to
`stacklok/homebrew-tap`. After it succeeds:

- **Bump `action.yml`'s `version` input default to the new tag and commit.**
  `action.yml` downloads a specific pinned release rather than building from
  source (see [`docs/08-github-action.md`](./docs/08-github-action.md)) — skip
  this step and the action keeps installing an old release indefinitely, with
  no error to flag it.

## Design history

[`audits/`](./audits/) holds dated audit + design-decision records (rationale for
the choices above) and the process for running new ones. The only known open
follow-up is a release-branch guard for `release.yml`, tracked as
[issue #1](https://github.com/stacklok/modelith/issues/1).
