---
sidebar_position: 10
title: Vendoring a model from another repository
description: Copy a model from another repository, reference its items, and keep the copy current as its origin moves on.
---

# Vendoring a model from another repository

[Imports](./06-schema-reference.md#imports) let one model reference an item
another one defines — but only across files that are already in your
repository. **Vendoring** is how a model from *somewhere else* gets there: you
fetch a copy, commit it, and modelith records where it came from.

Authenticate with the CLI for the host before your first import:

```sh
gh auth login # GitHub
az login      # Azure DevOps
```

Then copy the model's browser URL and import it. GitHub and Azure DevOps use
slightly different URL shapes:

```sh
modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
modelith deps import "https://dev.azure.com/acme/billing/_git/models?path=docs/payments.modelith.yaml&version=GBmain" docs/
```

## What you get

A copy of the file, byte for byte, with a **provenance header** added at the
top:

```yaml
# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
# modelith-vendored: DO NOT EDIT — this file is a copy. Change it at its origin.
# modelith-fetch: git
# modelith-origin: https://github.com/acme/billing
# modelith-path: docs/payments.modelith.yaml
# modelith-ref: main
# modelith-commit: 4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21
# modelith-imported: 2026-07-27
# modelith-digest: sha256:9a1f…
```

The header is a comment, not part of the schema — a model you write yourself
never has one, and never meets any of this.

| Key | What it records |
|---|---|
| `vendored` | That this file is a copy. Nothing enforces it; it is there so a person or an agent about to edit the file stops. |
| `fetch` | How to get it again. `git` today. |
| `origin`, `path`, `ref` | Where it came from and what to track. A tag in `ref` pins the copy; a branch follows it. |
| `commit` | The commit that last touched *this file* at that ref — so it does not move when unrelated commits land. |
| `imported` | When you fetched it. |
| `digest` | SHA-256 of the file with the header lines removed, so stamping the header does not change it. |

## Then add it to your model

`deps import` writes the file and stops. It does **not** edit your model's
`imports:` — it prints the line to add:

```yaml
imports:
  - ./docs/payments.modelith.yaml
```

The printed path is relative to the directory you ran the command in, because
that is the only thing `deps import` knows. An import path is relative to the
model that *declares* it, so if that model does not sit beside your working
directory, adjust it — exactly like any other import. Until you add that line,
the copy is an inert file that nothing reads.

That second step is deliberate. A vendored model is content someone else wrote
that will be rendered into *your* published Markdown, so `deps import` warns you
and leaves the decision — and the diff — visible.

:::warning[Only vendor from sources you trust]

A vendored model's prose ends up in your rendered `.md`. modelith escapes HTML
in prose fields, but the file is still somebody else's text landing in your
docs. Vendoring is designed for projects that already trust each other.

:::

## How a vendored file is treated differently

A provenance line marks the file as a copy whose home is elsewhere. `lint`
reports every provenance-header defect as a semantic error. It still suppresses
completeness findings for that copy, because those findings are about content
owned by its origin.

Its own `imports:` do not receive semantic diagnostics. A vendored model's
imports commonly name paths in its home repository that do not exist in yours.
Missing or broken nested edges stay silent, along with references that resolve
through them; readable local edges still participate in provenance verification.

Structural and other semantic checks still run. A vendored file that is not a
valid domain model, or whose digest no longer matches its header, fails `lint`.

`modelith render --check` applies its exemptions only to a vendored copy with a
clean provenance header. It skips a clean copy with no committed `.md`, and it
also skips a clean copy this version cannot render, such as one using a newer
schema version. If you commit the copy's `.md` for a deep link, `--check`
verifies it for staleness. A malformed header does not qualify for those
render-check exemptions, and `lint` reports the header error.

The [GitHub Action](./08-github-action.md) applies the same `lint` and
`render --check` behavior to every matching file.

## What it will not overwrite

The filename comes from the origin, so a copy can land on a file you already
have. `deps import` refuses two cases rather than clobbering them:

- **A model you wrote.** No provenance header means the file is yours, and no
  re-fetch could bring it back. Import into a different directory, or move the
  file aside first.
- **A copy of a *different* model with the same basename.** Two `payments.modelith.yaml`
  files from two repositories cannot share a directory; give them separate ones.

A copy from the *same* repository at a different path is refused too, because
modelith cannot tell a model that moved upstream from a second model whose file
happens to share a name. The message offers both remedies: delete the copy and
import again if it moved, or import into a different directory if they are two
different models.

Re-importing over an existing copy of the same model at the same path is the
ordinary refresh, and that goes through — it reports `replaced` rather than
`wrote`. Only the origin's *casing* is ignored in that comparison, because
GitHub treats an owner and repository name case-insensitively.

## Keeping the copy honest

Every `modelith lint` re-checks a vendored file against the digest in its own
header. If someone edits the copy, lint says so:

```
error [semantic] (root): this vendored file no longer matches the digest its
provenance header records (recorded sha256:9a1f…, computed sha256:2c7b…) — it
has been edited since it was imported. Restore it with `modelith deps update
docs/payments.modelith.yaml`, or delete the provenance header if the change is
a deliberate fork, which makes this repository the file's home.
```

Both remedies are real. `deps update` puts the copy back to what its origin
serves — and if the origin has not moved, that is byte for byte what the import
wrote. Deleting the header makes the file an ordinary model of yours — an
honest description of having forked it, and it re-enables the completeness
checks, because now it *is* your document.

This is drift detection, not a security boundary: anyone editing the file can
recompute the header. It catches the well-meaning typo fix, which is the thing
that actually happens.

When lint starts from an importing model, it follows locally readable imports
and verifies every vendored copy it reaches. A mismatch is reported against the
copy that needs repair, not its importer. This stays offline and does not add
new diagnostics for a nested import that cannot be read; lint does not become a
recursive semantic validator.

## Keeping the copy current

The section above is about your copy. This one is about the model it came from,
which moves on without you.

```sh
modelith deps check docs/*.modelith.yaml
```

```
docs/payments.modelith.yaml: up to date at v2.1.0
docs/ledger.modelith.yaml: stale at main — the origin is now at a91b0c3

checked 2 vendored copies, 1 stale
```

`deps check` writes nothing and exits non-zero when any copy is stale, so it
works as a scheduled CI job. `deps update` takes the same arguments and brings
the copies forward:

```sh
modelith deps update docs/ledger.modelith.yaml
```

```
docs/ledger.modelith.yaml: 4f2c1e9 → a91b0c3 at main

updated 1 of 1 vendored copy
```

Then read `git diff` to see what actually changed, and run `modelith lint`. An
item the copy used to define may have been renamed or removed upstream, which
breaks references in *your* model — `update` cannot see those, because it does
not know which of your models import the copy.

Both commands take file arguments, the same way `lint` does, and skip any file
with no provenance header. That means the glob you already lint works
unchanged; the closing line tells you how many files were skipped, so a glob
that matched none of your copies does not read as good news. To find them:

```sh
git grep -l '# modelith-vendored'
```

:::note[Refresh reaches GitHub only]

`deps check` and `deps update` use `gh`, so they cannot refresh a copy imported
from Azure DevOps. To take a newer version, import the file again from its
Azure DevOps browser URL. The import replaces the existing copy.

:::

### Two ways to track a model

Which one you are on is whatever `# modelith-ref:` records.

- **Tracking a branch.** `deps update` fetches whatever that branch has now.
  You get upstream's changes as they land, and `deps check` tells you when
  there are some.
- **Pinned to a tag.** `deps update` alone does nothing, because the tag still
  points where it did. Moving to a new version is explicit:

  ```sh
  modelith deps update --ref v2.2.0 docs/payments.modelith.yaml
  ```

  `--ref` re-pins one copy at a time. A single ref names a different version in
  every other repository, so it is refused with several files.

:::note[A pinned copy is always "up to date"]

`deps check` compares content against the ref your header records. On a tag,
that never changes, so a copy pinned to `v2.1.0` reports as up to date for as
long as `v2.1.0` exists — even after `v2.3.0` ships. modelith does not look for
newer releases, which is why every line of output names the ref it checked
against.

:::

### What "stale" means

A copy is stale when its origin serves **different content**, compared against
the digest in the copy's own header. It is not a commit comparison: a merge or
a whitespace-only touch upstream moves the commit without changing the model,
and that is not something you need to act on.

That has one consequence worth knowing: an upstream change to a `description:`
counts, because the digest covers the whole file. You will be told a
documentation-only change is waiting for you, and `git diff` after the update
is how you find out that is all it was.

`deps update` writes only where something changed, so running it over a glob
produces a diff exactly where one belongs. A copy that was hand-edited is not
holding what its origin serves, so it gets rewritten too, and the edits go —
make the change at the origin instead.

## Vendoring is one file, not a dependency tree

If the model you fetch imports models of its own, **those are not fetched**.
`deps import` tells you they exist, and `deps update` says the same thing again
if a refresh brings imports the copy did not have before:

```
Note: payments.modelith.yaml declares an import of its own (./ledger.modelith.yaml).
modelith vendors one file, not a dependency tree, and resolution is not
transitive — if you need items from those models, import them directly.
```

This matches how [resolution already
works](./06-schema-reference.md#imports): `payments.Thing` reaches only items
defined *directly* in the file bound to `payments`. If the item you want lives
one hop further away, vendor that model too and give it its own scope. The
linter says so at the reference site when it can tell:

```
attribute type "payments.Carrier" names no enum "Carrier" in
"./payments.modelith.yaml" — that model imports a model of its own, and
resolution is not transitive: if "Carrier" is defined in one of them, add that
model to this model's `imports:` too and reference it with its own scope.
```

Fetching a tree would mean a directory layout, rewritten paths, and an answer
for what happens when two models want different versions of the same third
model — a package manager, for a problem an explicit second `deps import`
already solves.

## Requirements and limits

- **Install the CLI for the host you import from.** For GitHub, install and
  authenticate the [GitHub CLI](https://cli.github.com). For Azure DevOps,
  install the [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/) and run
  `az login`. modelith delegates authentication to these tools.
- **Import from github.com or dev.azure.com.** An Azure DevOps URL has the form
  `https://dev.azure.com/<ORGANIZATION>/<PROJECT>/_git/<REPOSITORY>?path=<PATH>&version=GB<BRANCH>`.
  To request another host, [open an
  issue](https://github.com/stacklok/modelith/issues).
- **`lint` and `render` never touch the network**, whatever you pass them
  ([ADR-0011](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0011-network-boundary.md)).
  Everything under `modelith deps` is opt-in, and nothing else fetches.
- **No newer-release detection.** `deps check` tells you whether the ref you
  pinned still serves what you have. It does not tell you a newer tag exists,
  because deciding which tags count as newer means guessing at a versioning
  scheme modelith has no way to know.

The design and its trade-offs are
[ADR-0010](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0010-cross-model-references-by-vendoring.md),
[ADR-0015](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0015-vendoring-is-a-whole-file-copy.md),
and
[ADR-0016](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0016-staleness-is-a-content-comparison.md).
