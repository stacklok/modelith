# Cross-model references resolve against vendored copies

A model may declare a `scope:` slug and reference another model's items as
`slug.Name`. References resolve against a *copy of the other model committed to
this repo*, located by an explicit `imports:` path list, with provenance
recorded in a header comment. Fetching is an explicit, opt-in operation
delegated to `git`/`gh`; lint and render never touch the network.

## Context

A system outgrows one model, and the parts end up in different repos with
different lifecycles and owners. Today nothing lets one model refer to an item
defined in another. The workaround — define the same item in both models and
rely on the names matching — leaves the claim "these are the same" in prose,
where nothing resolves it and nothing checks it. That is issue #25.

The design pressure came from a real case: two models in separate repos sharing
a small enum, each defining it locally and each asserting the shared-ness in a
`description:`. The names agreed; the prose had already drifted. Nothing would
have caught a fourth value appearing on one side.

Two shapes were considered for that case. An *equivalence* form, where both
models keep a local definition and assert they match, was rejected: it requires
the format to carry a second mechanism, and it makes drift the normal state
rather than an error. The *reference* form — one canonical definition, others
point at it — is the whole design. Which model hosts the canonical copy, and
whether shared vocabulary deserves a third model of its own, is a modelling
choice for the author. The tool takes no position; the authoring skill offers
guidance.

## Decision

**Identity.** A model may declare a top-level `scope:` slug (kebab-case).
Uniqueness is conventional and scoped to the set of models that reference each
other; there is no registry, and none is planned. The slug is short because it
is written inline at every reference site, which is also why there is no
prefix-alias layer. `scope:` is optional — a model that nobody imports never
needs one — but it is required to *be* imported.

**Reference.** An item in another model is written `slug.Name`. In this change
only attribute `type` may be qualified; see "deferred" below.

**Resolution.** A model lists `imports:` as paths relative to itself. Each
listed file declares its own `scope:`, which is the single source of truth for
the slug — nothing is repeated, so nothing can disagree. There is no directory
convention and no scanning. Resolution does not recurse: an imported model's
own `imports:` are ignored, so only items defined directly in a listed file are
reachable. Two listed files declaring the same `scope:` is a lint error, as is
listing a file that declares none. Between two local models a collision is
fixed by renaming a `scope:`; between vendored copies of two upstreams that
chose the same slug there is no local remedy, because there is no alias layer
to bind one of them to a different name. Either way it is named rather than
silently resolved to whichever file was listed first.

**Local and vendored imports.** An import is either *local* — a model that
lives in this repo and is canonical here — or *vendored* — a copy of a model
whose home is elsewhere. Both are written the same way, as a path in
`imports:`. The difference is not declared: **a provenance header is the
signal.** A file carrying one is a copy, so it is verified against that header
and can be refreshed from origin. A file without one is canonical here, so
there is nothing to verify, nothing to refresh, and its absence of a header is
not a finding. Two models that are peers in one repo therefore import each
other with no ceremony at all, which is the common case and must stay the
cheapest one.

The cost is that a file copied in by hand, with no header, is indistinguishable
from one that is canonical. That is consistent with the posture below: this
mechanism detects drift, it does not police it.

**Vendoring.** A vendored import is a whole copy committed to this repo,
unmodified except for its provenance header. Whole-file rather than an
extracted subset, so that referencing something new from an already-imported
model is a local edit rather than another fetch.

**Provenance.** Origin, imported-at, and a surface digest live in a
`# modelith:` header comment in the vendored file. Not in the schema: an
`origin:` key would put a vendoring concept into the domain-model format that
every single-model user would then meet in the schema reference. Not a lock
file either: models commonly sit as a couple of peer files in a `docs/`
directory, and a lock at the repo root would be several directories from
anything it describes. The format already carries machine-read data in a header
comment (`# yaml-language-server: $schema=`), so this is an established shape
rather than a new one.

**Digest.** The surface digest covers structure and normative content — names,
ids, types, cardinalities, references, enum value names, and invariant
*statements*. It excludes documentation: `description`, `definition`, `note`,
`derivation`, scenario steps, `title`, comments, and key order. An invariant's
statement is the rule itself, so a reworded one is a real change to what a
consumer relies on; a `definition` is documentation of a name, and polishing it
should not page anybody. The digest answers "did anything change"; when an
explicit freshness check has both files in hand, it diffs them to report what.

**Checks.** `lint` re-verifies vendored files against their own headers by
default: offline, free, and it catches the hand-edit that a `DO NOT EDIT`
comment cannot prevent. Local imports have no header and so have nothing to
verify. Whether *origin* has moved is a separate question that `lint` never
asks; it belongs to `modelith deps check`, and a repo that wants aggressive
skew detection runs that in CI. The digest is a drift detector, not a security
boundary — anyone editing a vendored file can recompute the header, and that
is fine.

**Whose model is it.** A vendored file is not this repo's work, and
ownership-scoped diagnostics must not fire on it: completeness gaps, unused
enums and glossary terms, and unexercised entities are all findings about a
model its authors control. The provenance header is the signal here too. This
matters beyond taste, because the Action this repo ships lints every globbed
file with `--completeness` and optionally renders each one
(`action.yml:90-99`), so without this rule a vendored model turns a user's CI
red for gaps in someone else's document. Excluding a vendored directory from
the glob is the user's other lever, but it must not be the only one.

`render --check` is the same problem wearing different clothes: it fails when a
model's `.md` is absent or stale, and a vendored model arrives with neither
obligation — its rendered form belongs to its home repo. Vendored files are
therefore skipped by `render --check` rather than being required to carry a
committed `.md` that this repo would then have to regenerate whenever upstream
moved. Rendering one explicitly, by naming it, still works; that is how a deep
link into a vendored model's `.md` gets something to point at.

**A vendored file linted on its own.** Globs and editors will lint vendored
files directly, and such a file's own `imports:` point at paths in its home
repo that do not exist here. Unresolvable imports in a file that carries a
provenance header are skipped rather than reported; in a file without one they
are an error. This is the same signal doing the same job a third time.

**Fetching.** modelith does not implement network transport; it shells out
(ADR-0011). `gh` already solves authentication for private and internal
repositories, which the motivating models require. Commands are executed as an
argv array, never through a shell, and never sourced from a model file: a
vendored artifact must not be able to influence what runs.

**Render.** Rendered Markdown documents the dependency — an imports section
plus qualified type names, with relative deep links into the imported model's
rendered `.md` — but never expands imported definitions and never opens another
file. Output stays a pure function of the single model being rendered, so the
golden-fixture invariant holds and a vendored file changing cannot silently
rewrite an unrelated `.md`. Links to un-rendered models will dangle; that is
accepted, as is a link landing on the wrong heading when an entity and an enum
share a name, since both render as `### \`Name\`` into the same anchor.

This is a deliberate narrowing of ADR-0002, which holds that the Markdown
carries the full truth while the Mermaid ER is the lossy view. For imported
items the Markdown is lossy too: it names the dependency and links to it rather
than restating it. Determinism wins over completeness here, because a renderer
that reads other files is a renderer whose golden output changes when a file it
does not own changes.

**Commands.** Everything that manages an imported model lives under
`modelith deps`: `import` adds a model and stamps its header, `update`
refreshes one from origin, and `check` reports whether origin has moved. Only
`update` and `check` use the network, which is why they sit in a group of their
own rather than as flags on `lint`. Origins are assumed to be GitHub; other
hosts are a bridge to cross when someone needs one, and the origin field is a
string, so widening it later costs nothing.

The network boundary this rests on — that `lint` and `render` never reach the
network, and that commands which do delegate transport rather than implementing
it — is its own decision, recorded as ADR-0011.

## Deferred

Qualified references in `relationship.entity` and `subtypeOf` are not
supported. They already fail schema validation, since both fields carry
`pattern: ^[A-Z][A-Za-z0-9]+$`; lint gains a friendly error ahead of the raw
schema violation. The resolution mechanism is shared and would mostly work, but
two questions have no live instance driving them: whether the Mermaid ER draws
a foreign entity, and whether reciprocity and pairing checks apply across a
boundary. Relaxing the pattern later is backward compatible. Following
ADR-0007, the bar to lift this is a concrete modelling failure, not
expressiveness for its own sake.

Qualified names in prose are deferred with them. `lint`'s backticked-term check
(`lint.go:864-877`) warns on an unknown `` `Name` `` but will not resolve
`` `slug.Name` ``, so a cross-model reference in a `definition:` stays
unchecked. That is the pre-existing behaviour for anything non-PascalCase, and
this change does not improve it.

## Consequences

- The format gains `scope:` and `imports:`. The schema and the structs are held
  together by `TestSchemaStructSync`; `docs/06-schema-reference.md` is not
  covered by it and has to be updated by hand in the same change.
- Attribute `type` stops being an unconstrained string. A qualified type whose
  scope is not imported becomes an error; today such a value is silently
  treated as a primitive, because `lint.go`'s check only fires on PascalCase.
- `lint` and `render` gain the ability to read files other than their argument
  — `lint` to resolve imports, `render` only to the extent of emitting links.
- New command surface under `modelith deps`.
- A vendored model is a whole extra model in the repo, and every consumer of a
  model file has to learn the vendored-or-not distinction: `lint`, the
  completeness rules, `render --check`, the shipped Action, and this repo's
  own `TestADR_0009_NoSelfModel` allowlist, which fails on any `*.modelith.yaml`
  it does not know about.
- Nothing pins this ADR by test yet, because none of it is built. When it
  lands, `TestADR_0010_*` covers the digest's field selection, the
  no-transitive-resolution rule, and that ownership-scoped diagnostics skip a
  file with a provenance header; all three are decisions a test can hold. The
  deep-link anchor scheme needs a test too, since it depends on the renderer's
  heading format rather than on anything declared.
