---
sidebar_position: 9
title: Local Development
description: Building modelith, the Task workflow, and developing the Claude Code plugin locally.
---

# Local Development

For people working **on** modelith itself — the binary, the docs, or the Claude
Code plugin. For *using* the tool, start with the [overview](./index.md); for the
format, see the [schema reference](./06-schema-reference.md). Agents working on the
repo should also read [`CLAUDE.md`](https://github.com/stacklok/modelith/blob/main/CLAUDE.md),
which holds the repo layout and the CI-breaking conventions summarized at the
bottom of this page.

## Prerequisites

- **Go** (a recent stable release) — the binary and tooling are pure Go.
- **[Task](https://taskfile.dev)** — the task runner (`brew install go-task`).
- **`jq`** — used by the CI plugin check (`brew install jq`).
- **The `claude` CLI** — only needed to validate/develop the plugin locally.

## Building the binary

```sh
task build            # → ./bin/modelith
task install          # go install into GOBIN (on your PATH)
```

During development you can also skip the build and run from source —
`go run ./cmd/modelith lint <file>` — which is what the `lint-models` and
`render` tasks do.

## The Task workflow

The repo uses [Task](https://taskfile.dev). The one that matters before pushing:

```sh
task check
```

It runs the CI checks plus a local-only plugin validation. Run `task` with no
arguments to list every target.

| Command | What it does |
|---|---|
| `task build` | Build the binary into `./bin/modelith`. |
| `task install` | `go install` modelith into `GOBIN`. |
| `task test` | Run unit tests with cross-package coverage (writes `coverage.out`). |
| `task cover` | Show the per-function coverage report (run `task test` first). |
| `task vet` | Run `go vet`. |
| `task staticcheck` | Run staticcheck (pinned version). |
| `task lint-models` | Lint the example models (strict: completeness gaps are errors). |
| `task render` | Re-render the example models to Markdown. |
| `task render-check` | Verify the committed Markdown is up to date. |
| `task validate-plugin` | Validate the plugin with `claude plugin validate --strict` (needs the `claude` CLI). |
| `task check` | CI parity (vet, staticcheck, test, lint-models, render-check) plus `validate-plugin`. |

<details>
<summary>CI runs a lighter plugin check</summary>

`task check` runs the full `claude plugin validate ./plugin --strict` locally,
where you already have the `claude` CLI. CI instead does an equivalent `jq`-based
structural check (valid JSON, required fields, skill frontmatter present) so the
Go pipeline doesn't depend on the Claude Code CLI. Both gate the same thing.

</details>

## Developing the plugin locally

modelith ships a Claude Code plugin under [`plugin/`](https://github.com/stacklok/modelith/tree/main/plugin)
(skills that author, lint, and load domain models). The plugin files live here,
next to the binary they drive, so the skills and CLI version stay in lockstep.

To iterate on the plugin without publishing, point Claude Code straight at the
plugin directory:

```sh
claude --plugin-dir /path/to/modelith/plugin
```

You can pass `--plugin-dir` more than once to load several plugins at once. The
plugin's skills are namespaced under its name — invoke them as
`/modelith:domain-model-author`, `/modelith:domain-model-lint`, etc. Changes to
the plugin files are picked up on the next Claude Code session.

<details>
<summary>Already running the installed plugin? Uninstall it before testing locally</summary>

If `modelith@claude-community` is already installed, both copies load with
identical skill names and you can't tell which is active. Uninstall the managed
one before testing locally, and reinstall when you're done:

```sh
claude plugin uninstall modelith@claude-community
claude --plugin-dir /path/to/modelith/plugin   # test
claude plugin install modelith@claude-community
```

</details>

This same `--plugin-dir` trick is the cleanest way to try the authoring skills in
a **consuming** repo (one that holds a `*.modelith.yaml`) before publishing — launch
the session there with `--plugin-dir` pointing at your modelith checkout, rather
than copying skill files around.

Validate after any change to a manifest or skill:

```sh
task validate-plugin          # or: claude plugin validate ./plugin --strict
```

## Conventions that break CI (summary)

The full contract lives in [`CLAUDE.md`](https://github.com/stacklok/modelith/blob/main/CLAUDE.md);
the load-bearing ones:

- **The example is a golden fixture.** After any change to the renderer *or*
  `examples/example.modelith.yaml`, run `task render` to regenerate the committed
  `.md`. `task render-check` / CI fails on drift, and there's a golden test in
  `internal/render/markdown`. The example must also lint clean under
  `task lint-models` (strict).
- **Schema ↔ structs stay in sync.** `internal/schema/v1/modelith.schema.json`
  and `internal/model/model.go` are guarded by `TestSchemaStructSync`. Every
  object is `additionalProperties: false`.
- **The canonical schema URL appears in three places** (schema `$id`, the Go
  `URLFor`/`URL`, the example header); `TestURLConsistency` fails on drift.
- **The binary, not the schema, owns supported versions.** Adding a format
  version = a new `vN/` schema + a registry entry in `internal/schema`; never
  mutate a shipped version.
