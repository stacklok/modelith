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

Manages models **vendored** from other repositories — copies committed here and
marked with a provenance header. This is the only command group that uses the
network; `lint` and `render` never do.

### `modelith deps import <url> [dir]`

Fetches a model and writes it into `dir` (the working directory by default) as
a vendored copy.

```sh
modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
```

| Argument / flag | Meaning |
|---|---|
| `<url>` | The address of the file as it appears in a browser on github.com. |
| `[dir]` | Destination **directory**, defaulting to `.`. The filename always comes from the origin. |
| `--ref` | Ref to fetch, overriding the one in the URL. A tag pins the copy; naming a branch whose name contains a slash is also how you tell modelith where the ref ends and the path begins. |

A browse URL gives no way to tell a slashed ref from the path after it, so
modelith splits at the first segment. `--ref` fixes that split only when it
names the ref that is *in* the URL — it cannot both pin a different ref and
re-split the path. To pin, open the file on the ref you want and import that
URL. When a fetch fails, the error says how the URL was split.

Fetching is delegated to [`gh`](https://cli.github.com), which must be installed
and authenticated. The command writes the file and prints the `imports:` entry
to add — it does not edit your model. It refuses to overwrite a file at the
destination that is not an earlier copy of the same model.

### `modelith deps check <file>...`

Reports which vendored copies have fallen behind their origins. Writes nothing,
and exits non-zero when any copy is stale — or when one could not be reached,
since not being able to tell is not evidence that it is current.

```sh
modelith deps check docs/*.modelith.yaml
```

A copy is stale when its origin serves different content, compared against the
digest in the copy's own header. A commit that touched the path without
changing the file is not a change. Whether a copy *here* has been hand-edited
is a different question, and `lint` answers it offline.

Every line names the ref it checked against, because a copy pinned to a tag is
up to date for as long as that tag points where it did — modelith does not look
for newer releases.

### `modelith deps update [--ref <ref>] <file>...`

Brings vendored copies forward to what their origins serve.

```sh
modelith deps update docs/*.modelith.yaml
modelith deps update --ref v2.2.0 docs/payments.modelith.yaml
```

| Argument / flag | Meaning |
|---|---|
| `<file>...` | Vendored copies to update. Files with no provenance header are skipped. |
| `--ref` | Re-pin the copy to this ref. Applies to **one file**: a single ref names a different version in every other repository. |

A copy whose origin has not moved is left byte for byte alone, so running this
over a glob produces a diff only where something changed. A copy that was
hand-edited is not holding what its origin serves, so it is rewritten and the
edits go.

It writes the copies and nothing else — it does not edit any model's `imports:`
and it does not lint. Run `modelith lint` afterwards: an item a copy used to
define may have been renamed or removed upstream, which breaks references
`update` cannot see.

Both commands take file arguments the way `lint` does and skip files with no
provenance header, so the glob you already lint works unchanged. Find your
copies with `git grep -l '# modelith-vendored'`.

See [Vendoring a model from another
repository](./10-vendoring.md) for what the header records, how a vendored file
is linted differently, how the two tracking modes differ, and why vendoring
fetches one file rather than a tree.
