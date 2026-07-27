---
sidebar_position: 8
title: GitHub Action
description: Lint and verify domain models in CI.
---

# GitHub Action

Any repo can lint its domain model and verify the committed Markdown in CI by
referencing this repo as an action.

```yaml
# .github/workflows/domain-model.yml
name: Domain Model
on: [pull_request]

jobs:
  domain-model:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: stacklok/modelith@v1
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
| `version` | (pinned to a specific release) | `modelith` release to install, e.g. `v0.4.0`. |

Multiple files / globs:

```yaml
with:
  files: |
    docs/*.modelith.yaml
    services/**/model.modelith.yaml
```

## How it works

The action downloads the prebuilt `modelith` release binary for the runner's OS/arch
and verifies it against the release's published checksums before running it. The
`version` input defaults to a specific release pinned in `action.yml` — combined with
pinning your `uses:` reference to a commit SHA, this keeps CI runs reproducible: a
given action commit always installs the same `modelith` version.

## Vendored models in the glob

If your glob matches a model [vendored from another
repository](./10-vendoring.md), the action treats it as somebody else's
document:

- **Completeness gaps in it are not reported**, so `completeness: error` cannot
  fail your build over a model you did not write.
- **`check-rendered` skips it when you have not committed a `.md` for it**, so
  you are not asked to commit one whose home is another repository. If you *do*
  commit one — the way a deep link into a vendored model's Markdown gets a
  target — it is checked for staleness from then on.
- **`check-rendered` also skips a copy this modelith cannot render**, such as
  one written against a newer schema version. `lint` reports that, and repeating
  it here would fail your build twice over one problem you cannot fix in your
  own repository.

Both skips take a clean provenance header to claim. A file whose header has a
defect in it is checked like any model you wrote, so a stray `# modelith-`
comment cannot quietly switch the gate off.

Everything else still applies. A vendored file that is not a valid domain
model, or that has been edited since it was imported, fails the build — both
are about *your* repository's copy, and both are yours to fix.

## Regenerating the Markdown

The action **gates**; it does not commit. When `check-rendered` fails, run
`modelith render <file>` locally, commit the updated `.md`, and push — so the rendered
output is reviewed in the PR like any other change.
