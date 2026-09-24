---
sidebar_position: 7
title: The modelith CLI
description: Lint and render domain models from the command line.
---

# The `modelith` CLI

`modelith` validates domain-model YAML and renders it to Markdown. Use it directly
when you want to lint a model, regenerate its committed Markdown, or inspect the
schema. The [authoring agent](./02-getting-started.md) and CI use the same
commands.

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

`--stdout` cannot be combined with `--out` or `--check`.

If the model has [`imports`](./06-schema-reference.md#imports), the rendered
links to them are relative to wherever `-o` writes — `-o` a different
directory than the source and they still resolve, as long as the imported
model is rendered to *its* default location too. `--stdout` has no output file
to relativize against, so its links stay relative to the source.

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

## `modelith deps`

Manages vendored copies from other repositories. It is the only command group
that uses the network; `lint` and `render` run offline. See [Vendoring a model
from another repository](./10-vendoring.md) for provenance headers, ownership
semantics, and refresh behavior.

### `modelith deps import <url> [dir]`

Fetches a GitHub model and writes a vendored copy to `dir`, or the working
directory when omitted. The filename comes from the origin. It requires an
installed, authenticated [`gh`](https://cli.github.com) CLI and prints the
`imports:` entry to add; it does not edit your model.

| Argument / flag | Meaning |
|---|---|
| `<url>` | The address of the file as it appears in a browser on github.com. |
| `[dir]` | Destination directory. |
| `--ref` | Ref to fetch, overriding the ref in the URL. A tag pins the copy. |

When a branch or tag contains `/`, pass `--ref` only when it names that same ref in
the URL: it tells modelith where the ref ends and the file path begins. For
example, use `--ref release/v2` with a URL containing
`/blob/release/v2/docs/payments.modelith.yaml`. For an ordinary single-segment
ref in the URL, a different `--ref` works. But `--ref` cannot both select a
different ref and disambiguate a URL whose ref itself contains `/`; in that
ambiguous case, copy the browser URL for the file at the target ref.

```sh
modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
```

### `modelith deps check <file>...`

Checks vendored copies against their origins and exits non-zero when a copy is
stale or cannot be reached. It writes nothing and skips files without provenance
headers.

```sh
modelith deps check docs/*.modelith.yaml
```

### `modelith deps update [--ref <ref>] <file>...`

Updates vendored copies from their origins. `--ref` re-pins one copy to a tag or
branch; it accepts exactly one file. The command does not edit `imports:` or
lint the result.

```sh
modelith deps update docs/*.modelith.yaml
modelith deps update --ref v2.2.0 docs/payments.modelith.yaml
```

Use `modelith lint` after an update to find references that changed upstream.
