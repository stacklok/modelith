# The importer binds an import's scope

An `imports:` entry names the scope it binds, defaulting to the imported file's
basename with `.modelith.yaml` stripped. A model does not declare a name for
itself: the top-level `scope:` of ADR-0010 is gone. Supersedes ADR-0010's
**Identity** decision and the part of **Resolution** that made the imported file
the source of truth for the slug. The rest of ADR-0010 stands.

## Context

ADR-0010 put the slug in the imported file, so nothing was restated and nothing
could disagree. It also said the renderer documents each dependency with a deep
link into the imported model's rendered Markdown, and that it must never open an
imported file, so its output stays a pure function of the model it is rendering.

Those two cannot both hold. A link needs a scope *and* a path. Rendering a model
alone, the scopes are visible at the reference sites and the paths in `imports:`,
but the pairing between them lived in a file the renderer may not read. The gap
surfaced during implementation, not review: the lint layer was built and the
renderer had nothing to render.

The workarounds all made the tool guess. Matching a scope against an import's
filename, or building the link from the scope alone, each assumes a file is named
after its slug — an assumption nothing in the format enforces, whose failure mode
is a wrong link or a silently missing one.

## Decision

The binding moves to the importer, and `scope:` leaves the schema. `imports:` is
a union in the shape `actions` already established — a bare path, or
`{scope, path}` when the filename yields no usable slug or the obvious one is
taken:

```yaml
imports:
  - ./payments.modelith.yaml            # binds "payments"
  - scope: billing
    path: ./legacy/pay-v2.modelith.yaml # binds "billing"
```

This is deliberate aliasing, reversing issue #25's "no xmlns-style mapping" line.
Two models may bind the same file to different scopes, and the imported file has
no say in either. What it buys is that every consumer — linter, renderer, reader
— resolves a reference using only the file in front of it.

## Consequences

- The filename is the de facto identity, since the default comes from it.
  Consistent naming across repositories is conventional, not enforced. Accepted:
  the explicit form is the escape hatch, and a bare import whose filename is not
  a valid slug is a lint error that points at it.
- Two diagnostics from ADR-0010 are gone: an imported file that declares no
  scope, and a comparison between a declared scope and anything else. Duplicate
  bindings remain an error, now between two entries in one list rather than two
  files.
- Pinned by `TestADR_0012_ImporterBindsScope`: one file bound to different
  scopes by different importers resolves in both, and a top-level `scope:` is
  rejected.
