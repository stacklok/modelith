---
sidebar_position: 8
title: GitHub Action
description: Lint and verify domain models in CI.
---

# GitHub Action

Any repository can use this action to lint its domain models and verify committed
Markdown in CI. It supports Linux and macOS runners.

```yaml
# .github/workflows/domain-model.yml
name: Domain Model
on: [pull_request]

jobs:
  domain-model:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: stacklok/modelith@v0.4.0
        with:
          files: "model.modelith.yaml"
          completeness: warn
          check-rendered: true
```

## Inputs

| Input | Default | Description |
|---|---|---|
| `files` | — (required) | YAML files or globs, space-, comma-, or newline-separated. |
| `completeness` | `warn` | Treat completeness gaps as `warn` or `error`. |
| `check-rendered` | `true` | Verify the committed `*.md` matches the YAML. |
| `version` | `v0.4.0` | Published `modelith` release to install. Set another published release to select it. |

Multiple files / globs:

```yaml
with:
  files: |
    docs/*.modelith.yaml
    services/**/model.modelith.yaml
```

## How it works

The action downloads the prebuilt `modelith` release binary for the runner's OS
and architecture, verifies it against the release's published checksums, and runs
it. The `version` input defaults to `v0.4.0`; set it to another published release
when you need a different version. Pinning your `uses:` reference to a commit SHA
keeps CI runs reproducible: a given action commit installs the selected
`modelith` version.

## Vendored models in the glob

When the selected release supports vendored models, a glob can include a
[vendored model](./10-vendoring.md). A clean provenance header suppresses
completeness findings for that copy and lets `check-rendered` skip a missing
`.md` or a model this version cannot render. Structural and semantic findings,
including a provenance-header defect or a changed copy, still fail the action.

The default `v0.4.0` binary does not include vendored-model support. Select a
published release that does before relying on this behavior. See [Vendoring a
model from another repository](./10-vendoring.md) for provenance and render
semantics.

## Regenerating the Markdown

The action **gates**; it does not commit. When `check-rendered` fails, run
`modelith render <file>` locally, commit the updated `.md`, and push — so the rendered
output is reviewed in the PR like any other change.
