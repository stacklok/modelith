---
sidebar_position: 7
title: The modelith CLI
description: Lint domain models and render Markdown or offline HTML from the command line.
---

# The `modelith` CLI

`modelith` validates domain-model YAML and renders Markdown or a self-contained
HTML viewer. Use it directly when you want to lint a model, regenerate its
committed Markdown, explore entities in a browser, or inspect the schema. The
[authoring agent](./02-getting-started.md) and CI use the same commands.

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

Renders Markdown with an embedded Mermaid `erDiagram` by default, writing
alongside the input (`model.modelith.yaml` → `model.modelith.md`). With
`--format html`, it writes `model.modelith.html`: a self-contained, offline
viewer with searchable entities, relationships, details, and model-wide
sections. Open that file directly in a browser; it needs no server or assets.

The viewer starts in **Grid** layout. On wide screens, use the persistent **Entity details**
panel's **Collapse ›** button to give the graph the remaining width; use its narrow
**‹ Details** rail to reopen it. On smaller screens, the collapsed panel becomes a
full-width **Expand entity details** row. Selecting an entity while details are
collapsed updates the panel without reopening it. Its model-wide definitions appear below the graph in
**Invariants**, **Enums**, **Glossary**, **Scenarios**, and **Imports** tabs. The first
nonempty tab opens by default; use Left/Right arrows (or Home/End) to switch tabs,
then Tab into the selected panel to read its full-width, wrapped content.

Select **Flow down** or **Flow right** and
click **Arrange** to lay out relationship and is-a links in that direction.
These layouts use a simple, deterministic layering heuristic; they do not
minimize every crossing. Drag a node to adjust its position, or focus it and
press Alt+arrow keys. Click **Fit view** to show the current geometry without
moving nodes. **Arrange** replaces manual positions; reloading the file also
restores the initial grid. Positions are not saved. Drag the background or use
arrow keys to pan, and use the zoom buttons or + and − to zoom. Press 0 to fit.
Selecting an entity emphasizes its relationships and neighbors. Hover or click a
relationship, or Tab to its label, to highlight the label and both endpoints.
Press Escape in the graph to clear selection and relationship highlighting. Search always dims
nonmatching entities, even when selected or highlighted; clearing search restores
them. Edge hover/focus temporarily takes precedence over selection, which returns
when the pointer leaves or focus moves away. The Theme selector defaults to System
and can be set to Light or Dark for this page only. Selection, search, theme, and
positions reset on reload. Arrange crossfades briefly and Fit eases the camera;
reduced-motion preferences disable both transitions.

| Flag | Default | Description |
|---|---|---|
| `--format` | `markdown` | Output format: `markdown` or `html`. |
| `--out`, `-o` | input with `.md` or `.html` extension | Output path (the input's final `.yaml`/`.yml` replaced with the selected extension). |
| `--stdout` | `false` | Write to stdout instead of a file. |
| `--check` | `false` | Verify the committed output is up to date; non-zero exit on drift. |

`--stdout` cannot be combined with `--out` or `--check`.

For Markdown, if the model has [`imports`](./06-schema-reference.md#imports),
the rendered links to them are relative to wherever `-o` writes. They resolve
when the imported model is rendered to *its* default location too. `--stdout`
has no output file to relativize against, so its links stay relative to the
source. The HTML viewer shows qualified references as external nodes without
loading imported definitions.

The committed Markdown is the day-to-day read. `--check` is the CI gate that
keeps it honest:

```sh
modelith render model.modelith.yaml                 # regenerate Markdown
modelith render --check model.modelith.yaml         # fail if model.modelith.md is stale
modelith render --format html model.modelith.yaml   # write model.modelith.html
modelith render --format html --stdout model.modelith.yaml > viewer.html
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

Fetches a GitHub or Azure DevOps model and writes a vendored copy to `dir`, or
the working directory when omitted. The filename comes from the origin. GitHub
imports require an installed, authenticated [`gh`](https://cli.github.com) CLI.
Azure DevOps imports require the [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/)
and an `az login` session. The command prints the `imports:` entry to add; it
does not edit your model.

| Argument / flag | Meaning |
|---|---|
| `<url>` | The browser URL of a file on github.com or dev.azure.com. |
| `[dir]` | Destination directory. |
| `--ref` | Ref to fetch, overriding the ref in the URL. A tag pins the copy. |
| `--timeout` | Maximum duration for each delegated `gh` or `az` fetch. Defaults to `60s`; `0` disables the limit. |

GitHub browser URLs can be ambiguous when a branch or tag contains `/`. Pass
`--ref` only when it names that same ref in the URL: it tells modelith where the
ref ends and the file path begins. For example, use `--ref release/v2` with a
URL containing `/blob/release/v2/docs/payments.modelith.yaml`. For an ordinary
single-segment ref in the URL, a different `--ref` works. But `--ref` cannot
both select a different ref and disambiguate a URL whose ref contains `/`; in
that case, copy the browser URL for the file at the target ref.

```sh
modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
modelith deps import "https://dev.azure.com/acme/billing/_git/models?path=docs/payments.modelith.yaml&version=GBmain" docs/
```

### `modelith deps check <file>...`

Checks vendored copies against their origins and exits non-zero when a copy is
stale or cannot be reached. It writes nothing and skips files without provenance
headers. Copies from github.com and dev.azure.com are both checked.

| Flag | Default | Description |
|---|---|---|
| `--timeout` | `60s` | Maximum duration for each delegated `gh` or `az` fetch; `0` disables the limit. |

```sh
modelith deps check docs/*.modelith.yaml
```

### `modelith deps update [--ref <ref>] <file>...`

Updates vendored copies from their origins, for copies from github.com and
dev.azure.com alike. `--ref` re-pins one copy to a tag or branch; it accepts
exactly one file. The command does not edit `imports:` or lint the result.

| Flag | Default | Description |
|---|---|---|
| `--ref` | header's ref | Re-pin one copy to this ref (a tag or branch). One file only. |
| `--timeout` | `60s` | Maximum duration for each delegated `gh` or `az` fetch; `0` disables the limit. |

```sh
modelith deps update docs/*.modelith.yaml
modelith deps update --ref v2.2.0 docs/payments.modelith.yaml
```

Use `modelith lint` after an update to find references that changed upstream.
