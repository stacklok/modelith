# Vendoring is a whole-file copy, verified by a content digest

A vendored model is fetched whole, stamped with a provenance header comment,
and verified offline against a SHA-256 of its own bytes. Vendoring does not
recurse, the fetch is `gh`-only, and the trust warning prints rather than
blocks. Supersedes ADR-0010's **Digest** section; the rest of ADR-0010 stands.

## Context

ADR-0010 specified vendoring in full before any of it was built. Implementing
it, three of its answers turned out to cost more than the problem they solve,
measured against the threat model ADR-0014 settled: a model author already has
commit access and could edit the generated `.md` directly, so there is no
privilege boundary, and vendoring is expected to happen between projects that
already trust each other. The remaining gaps are correctness bugs, not
vulnerabilities, and the agreed answer to the injection risk is to *say so* at
the point of import rather than to armour the renderer further.

## Decision

**A content digest, not a surface digest.** The header carries a SHA-256 over
the file's bytes with every `# modelith-<key>: ` line removed. ADR-0010's
surface digest — structure and normative content, excluding `description`,
`definition`, `note`, `derivation`, scenario steps, `title`, and key order —
required a canonicalisation pass and imposed a permanent tax: every future
schema field would have to be classified surface-or-documentation, forever, or
the digest silently drifts. Checked against its two consumers, that buys very
little. For verify-on-lint, which is the job that must work offline with no
network, a whole-file digest is *strictly better*: the realistic accidental
edit is a teammate fixing a typo in someone else's model, which a surface
digest deliberately ignores. It loses only to `deps check`, where a
documentation-only upstream change will now report as a change — and `check`
has both files in hand and diffs them to report what moved, so a human judges.
A `surface:` key can be added later, with a real complaint driving it.

The digest is not redundant with `commit`. `commit` says what upstream had;
the digest says whether the bytes here still match what was fetched, which is
a question `lint` has to answer without a network.

**The header.** One namespaced key per comment line, mirroring the
`# yaml-language-server:` line already in every model:

```yaml
# modelith-vendored: DO NOT EDIT — this file is a copy. Change it at its origin.
# modelith-fetch: git
# modelith-origin: https://github.com/stacklok/some-repo
# modelith-path: docs/payments.modelith.yaml
# modelith-ref: main
# modelith-commit: 4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21
# modelith-imported: 2026-07-27
# modelith-digest: sha256:9a1f...
```

Parsing and stripping are the *same* rule — lines matching
`^# modelith-<key>: ` — so the two cannot disagree, which matters because the
digest is defined by the strip. Such a line below the leading comment block is
an error. The `vendored` key carries the DO NOT EDIT banner as its value:
nothing enforces it, but an agent reading the file will pause on it, and
making the banner a key rather than free text keeps the single strip rule.

`fetch:` declares the method and scopes which other keys apply — `git` uses
`origin`, `path`, `ref`, and `commit`; a future `https` method would carry
`origin` alone and lean on the digest as its only integrity anchor. An unknown
`modelith-*` key or an unknown `fetch:` value is an error rather than being
ignored, which is the right posture pre-release even though it means an older
binary rejects a newer header.

**What the header suppresses.** A provenance header suppresses the
completeness category and import-resolution errors, and nothing else.
Structural and semantic checks still run: a vendored file that is not valid
modelith breaks *this* repo's build and is this repo's problem. ADR-0010's
list of ownership-scoped diagnostics — no invariants, no scenario exercises an
entity, unused glossary term, unused enum — is exactly `CategoryCompleteness`,
so the rule needs no new classification. Semantic *warnings* about someone
else's authoring stay: they never fail a build, and suppressing them would
require a second, fuzzier judgement about which semantic checks are
ownership-scoped.

Suppressing an unresolvable import must suppress its downstream consequence
too. A vendored file's own `imports:` point at paths in its home repo; if the
references into those scopes still errored, every vendored file that has
imports would be broken here and the suppression would be worthless.

A digest mismatch is an error, not a warning: the file lies about its own
provenance, and both remedies are cheap and honest — re-run `deps update`, or
delete the header, which converts it to a canonical local file and is an
accurate description of having forked it. The diagnostic names both.

**Vendoring does not recurse.** ADR-0010 already made *resolution*
non-transitive: `scope.Name` reaches only items defined directly in the file
bound to that scope. Fetching stays non-transitive to match. Referencing an
item that a vendored model merely uses rather than defines is a lint error
whose remedy is to vendor that model directly and bind it to its own scope,
and `deps import` says at fetch time when the file it fetched declares imports
of its own.

The closure was considered and rejected on cost. It requires a vendor
directory layout, rewriting import paths on fetch — which breaks the
unmodified-copy invariant that both the digest and a clean `deps update`
overwrite rest on — a dedupe policy for diamond dependencies, and an answer to
a conflict ADR-0012 makes unavoidable: the *importer* binds the scope, so a
transitively vendored file can arrive bound to one slug by its parent and
another by this repo. Resolving that needs a lock file and an alias layer,
both of which ADR-0010 rejected. It is also consistent with the reason
vendoring is whole-file: referencing something new from an already-imported
model is a local edit, and referencing something from a *different* model is
another explicit import.

**`gh` is the only transport.** `gh` already solves authentication for the
private and internal repositories the motivating models require, and it
resolves content and commit in two API calls. A `git` clone path would work
with any remote but is a second code path with sparse-checkout handling for a
case with no live instance, which is the bar ADR-0007 sets. A non-GitHub
origin is an error that asks the user to file an issue, so the first real user
becomes the signal to build it. `fetch: git` names what the origin *is* — a
file at a path in a git repo at a ref — not which binary fetched it, so a
`git` fallback or an `https` method slots in without a header migration.

**The warning prints; it does not block.** ADR-0014 requires `deps import` and
`deps update` to warn that a vendored model is untrusted content that will be
rendered into published Markdown. An interactive confirmation is the wrong
shape: an agent usually drives these commands, so a prompt is either
auto-answered theatre or a wedged non-interactive CI run. Warn-and-proceed
means the fetch has happened by the time the warning is read, which is
acceptable because the file is inert until it is added to `imports:` — and
`deps import` deliberately does not edit `imports:`, so that second, manual
step is the real gate.

## Consequences

- `deps import` writes a file and prints the `imports:` snippet to paste.
  Rewriting a user's YAML in place would cost comment and formatting
  fidelity and would force the command to answer which model it is importing
  *into* when a repo holds several.
- No schema change. `imports:` already exists and a header comment is not
  schema, so `TestSchemaStructSync` and the schema-URL consistency tests are
  not in play.
- Pinned by `TestADR_0015_*`: the digest covers file bytes minus the header
  lines, a header suppresses completeness and nothing else, an unknown
  `fetch:` value is an error, and `deps import` fetches no transitive imports.
- An unbalanced code fence in a vendored model's prose still corrupts the
  importing repo's rendered Markdown (issue #39). Vendoring strengthens the
  case for that rule, since it is where someone else's prose lands in your
  published document, but it shares no code with this change and is a visibly
  broken document rather than a privilege boundary.
