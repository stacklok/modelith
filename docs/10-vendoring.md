---
sidebar_position: 10
title: Vendoring a model from another repository
description: Copy a model from another repository, keep track of where it came from, and reference its items.
---

# Vendoring a model from another repository

[Imports](./06-schema-reference.md#imports) let one model reference an item
another one defines — but only across files that are already in your
repository. **Vendoring** is how a model from *somewhere else* gets there: you
fetch a copy, commit it, and modelith records where it came from.

```sh
modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
```

That URL is the address of the file as it appears in your browser on
github.com — open the model on GitHub and copy the address bar.

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

Once a file carries a provenance header, modelith knows it is not your work.
Two things change, and nothing else:

- **Completeness findings are suppressed.** Missing invariants, entities no
  scenario exercises, unused enums and glossary terms are gaps in a document
  its own authors control. Without this, the [GitHub
  Action](./08-github-action.md) — which lints every matched file — would fail
  your build over someone else's model.
- **Its own `imports:` raise nothing.** A vendored model's imports name paths
  in *its* repository, which do not exist in yours. Those are skipped, along
  with the references that resolve through them.

**Structural and semantic checks still run.** A vendored file that is not a
valid domain model breaks your build, and that is your problem to solve — by
fetching a different ref, or by talking to whoever owns it.

`modelith render --check` skips a vendored file that has no committed `.md`:
its rendered Markdown belongs to its home repository, so you are not asked to
commit one. Rendering a vendored model by naming it still works, which is how a
deep link into it gets something to point at — and once you commit that `.md`,
`--check` treats it like any other and tells you when refreshing the copy has
left it stale.

The one thing `--check` will not do is fail over a vendored model this modelith
cannot render at all — one written against a newer schema version, say. It says
it skipped it and moves on; `modelith lint` is where that is reported, once.

## What it will not overwrite

The filename comes from the origin, so a copy can land on a file you already
have. `deps import` refuses two cases rather than clobbering them:

- **A model you wrote.** No provenance header means the file is yours, and no
  re-fetch could bring it back. Import into a different directory, or move the
  file aside first.
- **A copy of a *different* model with the same basename.** Two `payments.modelith.yaml`
  files from two repositories cannot share a directory; give them separate ones.

Re-importing over an existing copy of the *same* model is the ordinary refresh,
and that goes through — it reports `replaced` rather than `wrote`.

## Keeping the copy honest

Every `modelith lint` re-checks a vendored file against the digest in its own
header. If someone edits the copy, lint says so:

```
error [semantic] (root): this vendored file no longer matches the digest its
provenance header records (recorded sha256:9a1f…, computed sha256:2c7b…) — it
has been edited since it was imported. Refresh it with `modelith deps import
https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml`, or
delete the provenance header if the change is a deliberate fork, which makes
this repository the file's home.
```

Both remedies are real. Re-running `deps import` replaces the copy with what
upstream has now. Deleting the header makes the file an ordinary model of
yours — an honest description of having forked it, and it re-enables the
completeness checks, because now it *is* your document.

This is drift detection, not a security boundary: anyone editing the file can
recompute the header. It catches the well-meaning typo fix, which is the thing
that actually happens.

## Vendoring is one file, not a dependency tree

If the model you fetch imports models of its own, **those are not fetched**.
`deps import` tells you they exist:

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

- **`gh` must be installed and authenticated.** modelith implements no network
  transport of its own; it delegates to the [GitHub
  CLI](https://cli.github.com), which already solves authentication for private
  and internal repositories.
- **GitHub only, for now.** A URL on another host is an error that asks you to
  [open an issue](https://github.com/stacklok/modelith/issues). That is not a
  brush-off: the header records *how* it was fetched, so adding another
  transport is straightforward — what is missing is a real user to build it
  for, and an issue is how you become one.
- **`lint` and `render` never touch the network**, whatever you pass them
  ([ADR-0011](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0011-network-boundary.md)).
  Everything under `modelith deps` is opt-in, and nothing else fetches.

The design and its trade-offs are
[ADR-0010](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0010-cross-model-references-by-vendoring.md)
and
[ADR-0015](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0015-vendoring-is-a-whole-file-copy.md).
