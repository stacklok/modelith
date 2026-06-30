---
sidebar_position: 7
title: The modelith CLI
description: Lint and render domain models from the command line.
---

# The `modelith` CLI

`modelith` lints domain-model YAML and renders it to Markdown. It's the **engine
the [authoring agent](./02-getting-started.md) and CI run for you** — you'll
rarely invoke it directly. This page is the reference for when you do: every
command, flag, and the one-time install.

## Installation

Install the latest release with Homebrew:

```sh
brew install stacklok/tap/modelith
```

Or download a prebuilt binary from the
[Releases page](https://github.com/stacklok/modelith/releases), build from
source with `go install`, or build from a checkout with `task build`:

```sh
go install github.com/stacklok/modelith/cmd/modelith@latest
```

## `modelith lint`

```sh
modelith lint <file>...
```

Validates one or more files across three layers — structural (JSON Schema),
semantic (cross-references), and completeness (advisory gaps).

| Flag | Default | Description |
|---|---|---|
| `--completeness` | `warn` | Treat completeness gaps as `warn` or `error`. |
| `--format` | `text` | Output format: `text` or `json`. |

Exit code is non-zero when there are errors, or when completeness gaps exist and
`--completeness=error`. `--format json` is for CI annotations.

```sh
modelith lint examples/example.modelith.yaml
modelith lint --completeness error --format json model.modelith.yaml
```

## `modelith render`

```sh
modelith render <file>
```

Renders the model to a single Markdown document with an embedded Mermaid
`erDiagram`. By default it writes alongside the input (`model.modelith.yaml` →
`model.modelith.md`).

| Flag | Default | Description |
|---|---|---|
| `--out`, `-o` | input with `.md` extension | Output path (the input's `.yaml`/`.yml` replaced with `.md`). |
| `--stdout` | `false` | Write to stdout instead of a file. |
| `--check` | `false` | Verify the committed output is up to date; non-zero exit on drift. |

The committed Markdown is the day-to-day read. `--check` is the CI gate that
keeps it honest:

```sh
modelith render model.modelith.yaml          # regenerate
modelith render --check model.modelith.yaml  # fail if model.modelith.md is stale
```

## `modelith schema`

Prints the canonical JSON Schema to stdout — handy for editor setup or piping
into another validator.

```sh
modelith schema > modelith.schema.json
```
