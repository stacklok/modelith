---
sidebar_position: 6
title: Schema Reference
description: Field-by-field reference for the domain-model YAML format.
---

# Schema Reference

The canonical schema URL is `https://modelith.sh/schema/domain-model/v1.json`
(JSON Schema, draft 2020-12). Add it to your file as a header:

```yaml
# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
```

:::note[Schema URL not yet live]

Serving the schema at `modelith.sh` is a roadmap item. Until it is, the URL
doesn't resolve, so you won't get editor autocomplete from the header — but the
header is harmless and `modelith lint` validates the file regardless (it embeds
the schema). Print the schema any time with `modelith schema`.

:::


## Top level

| Field | Type | Required | Notes |
|---|---|---|---|
| `kind` | string | yes | Must be `DomainModel`. |
| `version` | string | yes | Schema revision. Currently `v1`. |
| `title` | string | no | Heading used when rendering. |
| `description` | string | no | One-paragraph summary. |
| `imports` | list | no | Other model files whose items this one references, each a path relative to this file. See [Imports](#imports). |
| `glossary` | map | no | Ubiquitous-language terms that aren't entities. See [Glossary](#glossary). |
| `enums` | map | no | First-class enumerated types. See [Enum](#enum). |
| `entities` | map | no | Keyed by canonical PascalCase entity name. If present, must contain at least one entity. |
| `scenarios` | list | no | Narratives that exercise the model. |
| `invariants` | list | no | Model-level rules that span several entities. Same shape as entity invariants. See [Invariant](#invariant). |

`kind` and `version` make the file **self-describing**: tooling dispatches on
them, and they let the format evolve without guesswork.

## Glossary

`glossary` defines the ubiquitous-language terms that are **not** entities —
roles (`Owner`, `Member`), states of being, domain nouns. Each key is the term
(PascalCase) and the value is its definition. Defining a term here makes it part
of the checked vocabulary rather than something the linter only infers from
incidental use.

```yaml
glossary:
  Owner: "A `User` with full control of a `Project` — can transfer ownership and archive it."
  Member: "A `User` granted access to a `Project` without ownership rights."
```

A term used as a relationship `role` or a scenario `actor` should be defined
here; the linter warns on a role term that resolves to neither an entity nor a
glossary term, and flags a glossary term nothing references.

## Enum

`enums` defines first-class enumerated types, keyed by PascalCase name. An
attribute selects one by naming it in its `type` (rather than burying values in
a `"enum[...]"` string, which is unparseable and uncheckable).

```yaml
enums:
  ProjectStatus:
    description: "Lifecycle state of a `Project`."
    values:
      - name: active
        definition: "In normal use; `Policies` can be enabled."
      - name: archived
        definition: "Retired and read-only."
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `description` | string | no | What the enumerated type represents. |
| `values` | list | yes | Each value has a `name` and optional `definition` so a state like `active` has one agreed meaning. |

Enums **name** the states; the legal *transitions* between them live in invariants
and action `preserves`, not a separate transitions construct — that's a deliberate
omission to keep the format light. (E.g. "can't archive while policies are enabled"
is an invariant the `archive` action preserves, not an edge in a state machine.)

## Entity

Each key under `entities` is the entity's canonical name (PascalCase, e.g.
`Project`). Its value:

| Field | Type | Required | Notes |
|---|---|---|---|
| `definition` | string | yes | Two to four sentences: what it is, what it is not. |
| `subtypeOf` | string | no | Names the entity this one is a kind of (an is-a link). Must reference a defined entity. |
| `relationships` | list | no | See [Relationship](#relationship). |
| `attributes` | list | no | See [Attribute](#attribute). |
| `actions` | list | no | Mutations the system exposes. See [Action](#action). |
| `invariants` | list | no | Rules that must always hold. See [Invariant](#invariant). |
| `derived` | boolean | no | True if the entity is wholly computed from other state rather than persisted — never stored, recomputed on demand. |
| `derivation` | string | no | How a derived entity is computed. Unlike an attribute's `derivation`, this is optional even when `derived` is true — the `definition` often already explains it. |

Mark an entity `derived` when it has no persisted identity of its own — a
computed report, a query result, a set of findings recomputed on every read.
That distinction is easy to lose once the entity has relationships and
attributes like any other, and the rendered diagram would otherwise draw it
exactly like a stored one. The linter warns if a `derived` entity is the
target of an `ownership: owned` relationship — composing an ephemeral thing is
usually a modeling error. The Mermaid ER diagram does not visually distinguish
derived entities — per-entity styling is unverified across the Mermaid
versions in play, so the ER stays a deliberately lossy view; the Markdown text
is the source of truth.

Use `subtypeOf` for generalization — when one entity *is a kind of* another
(a `Card` is a `PaymentMethod`). The child declares it, and it must name a
defined entity; the linter errors on an undefined parent or a cycle. A parent's
invariants are understood to cover its subtypes, so a subtype that adds no rule
of its own is not flagged for having no invariants. The Mermaid ER diagram does
not draw the is-a link — erDiagram has no generalization notation, so the
hierarchy lives in the rendered Markdown (each child names its supertype and
each parent lists its subtypes), a deliberately lossy ER per the same principle
as derived entities.

## Relationship

| Field | Type | Required | Notes |
|---|---|---|---|
| `entity` | string | yes | Target entity name. Must reference a defined entity. |
| `cardinality` | string | yes | Written `left:right` (see below). `1:1`, `1:n`, `n:1`, `n:n` are the common shorthands. |
| `symmetric` | boolean | no | The relationship carries no inherent order: `(a, b)` is the same as `(b, a)`. Only valid on a self-referential relationship or one whose target side is more than one. |
| `role` | string | no | The **short** role the related entity plays (`Owner`, `Predecessor`) — ideally a glossary term. Backtick entity and glossary names. It is the only label the diagram draws, so prose belongs in `note`; the linter warns on a role that reads as a sentence. |
| `ownership` | enum | no | Is the related entity *part of* this one? `owned` = it can't exist independently (composition: created within, and deleted with, this entity); `referenced` = an independent entity this one points at. Omitted ⇒ `referenced`. |
| `note` | string | no | Freeform note. |

**Cardinality grammar.** Each side is a multiplicity: `1` (exactly one), `n`
(zero or more), an exact count like `2`, or a range like `0..1`, `1..n`, `0..5`.
So `1:2` is exactly two, `1:0..1` is optional, and `1:1..n` is at least one. The
rendered Mermaid diagram has no numeric cardinality, so an exact or bounded
count draws as the nearest crow's-foot (one-or-many for `1:2`); the precise
count stays in this table and the `role`. Combine `symmetric: true` with an
exact count — `1:2 symmetric` — to declare an unordered pair.

You can declare a relationship from one side or both. If you declare it from
both — `Project` lists `Policy` *and* `Policy` lists `Project` — the
cardinalities must be inverses (`1:n` one way ⇒ `n:1` the other; `1:1` and `n:n`
invert to themselves). The linter errors on a contradiction, and the renderer collapses
a matching pair into a single edge. Declaring it once is fine; the renderer
shows the edge either way.

When there's an intuitive **parent** — the entity that owns or contains the
other, or sits on the "one" side of a one-to-many — prefer declaring the
relationship there (e.g. on `Project`, not `Policy`). It keeps each link in one
obvious place and reads the way the domain does. Declare from both ends only
when both views genuinely add clarity.

**How a relationship draws.** Three conventions, recorded in
[ADR-0008](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0008-er-diagram-conventions.md)
and covered in full in [Reading the Diagrams](./04-reading-the-diagrams.md):

- **`ownership` is the line style** — solid (identifying) for `owned`, dashed
  (non-identifying) for `referenced` and for an omitted `ownership`. It costs no
  label space. Ownership belongs to the relationship rather than the end that
  declared it, so a parent's `owned` and the child's `referenced` fold into one
  solid line when their cardinalities are inverses and each end declares it
  once — even when the two ends name different roles, in which case the owning
  end's role labels the line and the other stays in the Markdown. Declarations
  the renderer can't reduce to one relationship draw as separate lines: mutual
  `owned` is a lint error, and a pairing it can't resolve is a lint warning.
- **`role` is the only label** — `ownership` and `cardinality` are never written
  on a line. Keep the role short; put the explanation in `note`.
- **A self-referential relationship becomes a row inside the entity's box**
  (`Project self "1:0..1 — Predecessor"`), not a line looping back on it.
  Mermaid's ER layout has no self-loop handling, and the arc it draws swamps the
  diagram. The row carries the declared cardinality in full (both sides — the
  two ends of the line it replaces), `owned` when owned, and the role.

## Attribute

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Attribute name. |
| `type` | string | yes | A **primitive** (lowercase, e.g. `string`, `integer`, `boolean`, `timestamp`) or the **PascalCase name of a defined [enum](#enum)**. A PascalCase type that names no enum is flagged. |
| `description` | string | no | |
| `derived` | boolean | no | True if computed from other state rather than stored. Forces `derivation`. |
| `derivation` | string | no | How a derived attribute is computed. Required when `derived` is true. |

Attributes are the properties that matter for reasoning about the entity — not
every database column. Mark computed values `derived` so they aren't mistaken
for stored columns.

## Action

Each item under an entity's `actions` is either a **bare name** or a
**structured object**. Use the object form to tie an action to who performs it
and the invariants it must preserve.

```yaml
actions:
  - create                       # bare
  - name: archive                # structured
    actor: Owner                 # an entity or glossary term
    preserves: [at-least-one-owner]   # invariant ids
    description: "Retire the project."
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | The action name. |
| `actor` | string | no | Who performs it — a defined entity or glossary term. |
| `preserves` | list of string | no | Ids of invariants this action must preserve. |
| `description` | string | no | |

## Invariant

Each invariant carries a stable `id` and a `statement`. References (scenario
`invariants_touched`, action `preserves`) point at the **id**, so rewording the
statement never silently breaks them.

```yaml
invariants:
  - id: at-least-one-owner
    statement: "Must have at least one `Owner` at all times"
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | Stable identifier, lowercase kebab-case. Unique across the model. |
| `statement` | string | yes | The rule. Short, declarative, testable. Backtick entity names. |

An invariant can be declared in one of two places:

- **On an entity** (`entities.<X>.invariants`) — for a rule with a clear single
  owner, e.g. "a `Project` must always have at least one `Owner`."
- **At the top level** (`invariants`, sibling to `entities`) — for a rule that
  spans several entities and has no natural owner, e.g. "when a `Project` is
  archived, none of its `Policies` remain enabled." Putting such a rule on one
  arbitrary entity would misattribute it.

Both forms use the identical shape and **share one id namespace**: ids must be
unique across entity-level and model-level invariants alike, and a
`invariants_touched` / `preserves` reference resolves against either scope. The
renderer emits model-level invariants in a top-level `## Invariants` section;
entity-level ones render with their entity.

## Scenario

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Short title. |
| `actors` | list of string | no | Entity names or glossary roles involved. Ad-hoc participants (e.g. `TargetUser`) are allowed and not required to be glossary terms. |
| `steps` | list of string | yes | Ordered steps. Backtick entity names. |
| `invariants_touched` | list of string | no | **Ids** of invariants this scenario exercises. Each must reference a declared invariant. |

A scenario is a diagnostic, not a backlog item: it tests whether the entities
and actions actually hang together. If writing one reveals an invariant that
can't be satisfied — or that doesn't exist yet — fix the model, not the scenario.

## Imports

Once a system outgrows one model, two models end up needing the same concept —
typically a shared enum. Defining it in both and asserting in prose that they
match leaves nothing to check, and they drift. Instead, one model owns the
definition and the other **references** it.

List the other model's file, and write `scope.Name` where you reference it:

```yaml
# garage.modelith.yaml
kind: DomainModel
version: v1
imports:
  - ./payments.modelith.yaml
entities:
  Visit:
    definition: One car's stay in the garage.
    attributes:
      - name: settledWith
        type: payments.PaymentMethod
```

The imported file needs no cooperation at all — it is an ordinary model that
happens to define a `PaymentMethod` enum. **The scope is bound by the model
doing the importing**, and defaults to the file's basename with
`.modelith.yaml` stripped: `./payments.modelith.yaml` binds `payments`.

Name it explicitly when the filename yields no usable slug, or when the obvious
one is already taken:

```yaml
imports:
  - ./payments.modelith.yaml            # binds "payments"
  - scope: billing                      # binds "billing"
    path: ./legacy/pay-v2.modelith.yaml
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `scope` | string | yes (explicit form) | The slug written before an item's name. Lowercase kebab-case (`^[a-z][a-z0-9-]*$`). |
| `path` | string | yes (explicit form) | Path to the model file, relative to *this* one. Relative, and free of control characters. |

A bare string is the same thing with the scope derived — `./x.modelith.yaml` is
exactly `{scope: x, path: ./x.modelith.yaml}`. If the filename doesn't yield a
valid slug (`./Pay Ments.yaml`), the linter says so and points at the explicit
form.

Six rules are worth knowing before you use this:

- **A dot means a cross-model reference.** An attribute `type` containing one
  has to be exactly `scope.Name` — one dot, a slug before it, a PascalCase item
  name after. `payments.v2.PaymentMethod`, `.PaymentMethod` and `payments.` are
  errors that say which way they are malformed, not types quietly read as
  primitives.
- **The binding is local.** Two models may import the same file under different
  scopes, and neither one's choice is visible to the other. Two imports binding
  the *same* scope in one model is an error — give one of them an explicit,
  different scope.
- **Resolution does not recurse.** Only items defined *directly* in a listed
  file are reachable. If `garage` imports `payments` and `payments` imports
  `shipping`, `shipping.Carrier` is not available in `garage` — import it
  there too. Mutual imports (`a` lists `b`, `b` lists `a`) are therefore legal
  and terminate.
- **Only an attribute `type` may be qualified.** Cross-model references in
  `relationship.entity` and `subtypeOf` are not supported; the linter says so
  plainly rather than letting the name-pattern rejection speak for it. Whether
  the ER diagram should draw a foreign entity — and how reciprocity would work
  across a boundary — has no answer yet, and no live model needs one.
- **Nothing is fetched.** `imports` names files that are already in your
  repository; `lint` and `render` never touch the network
  ([ADR-0011](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0011-network-boundary.md)).
- **An import cannot leave the repository.** `..` is fine, but only as far as
  the repository holding the model — the nearest directory above it with a
  `.git` entry. Past that it is an error naming where the path resolved to and
  what the root is. Symlinks are followed first, so a link pointing out is
  caught too. **With no repository anywhere above the model, the root is the
  model's own directory** — outside a repository the tool cannot tell how far
  your project extends, so it assumes the least, and even a sibling directory
  is out of reach.

Rendered Markdown names each import and links it to that model's rendered `.md`,
and a qualified type links straight to the item's heading there. The renderer
never opens an imported file, so a link points at where the Markdown *would* be:
render the imported model too, or the link dangles.

The linter reports a qualified type that doesn't resolve as an **error**, while
an *unqualified* PascalCase type that names no enum is only a **warning**. The
asymmetry is deliberate: `PaymentMethod` might be a primitive the author
invented, so the linter can only suggest; `payments.PaymentMethod` can be
nothing but a cross-model reference, so failing to resolve it is a broken
reference.

The repository boundary keeps a model from an untrusted source probing the
filesystem of whatever machine lints it — the four answers an import can produce
(resolves, missing, unreadable, not a model) are otherwise a usable oracle. It
does not stop the same probing *within* the repository, and that is accepted:
anyone who can already commit a file to your repository has better options than
a lint diagnostic.

The full rationale, including where this is heading, is
[ADR-0010](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0010-cross-model-references-by-vendoring.md),
and the boundary itself is
[ADR-0013](https://github.com/stacklok/modelith/blob/main/project-docs/adr/0013-imports-confined-to-the-repository.md).

## What this format deliberately leaves out

modelith is a light, agent-authored subset of domain-driven design, not a full
DDD notation. Several classic DDD concepts are left out on purpose. Knowing what
is *not* here is as useful as knowing what is.

- **Aggregates and aggregate roots.** There is no first-class aggregate
  boundary. A consistency boundary is expressed by the invariants that must hold
  and the entity that owns them, not by a declared aggregate. Deliberate: the
  boundary lives in the rules, which the format already captures.
- **Value objects.** There is no value-object type. Model a value-shaped concept
  as an owned entity or as attributes on its owner. The parking-garage example
  models `Ticket` this way and names the tension. First-class structured value
  types were considered and set aside: named, typed fields define record
  structure — an implementation detail this format abstracts away
  ([issue #11](https://github.com/stacklok/modelith/issues/11)).
- **Domain events.** There is no event construct. A state change is expressed as
  an `action` plus the invariants it `preserves`, and enums **name** states
  while invariants govern the legal transitions between them. Deliberate, and
  consistent with why enums carry no transition edges.
- **Bounded contexts and context maps.** One model is one context. There is no
  construct for declaring a context boundary or mapping a concept from one
  context onto another's. What there is, is a narrow reference:
  [imports](#imports) let one model name an item another defines, so shared
  vocabulary has one definition. Everything else about how two contexts relate
  stays outside the format.

These omissions keep the format small enough for an agent to author reliably and
for a human to read in one sitting. Any of them can become a roadmap item if a
real model needs it; none is here yet beyond what is linked above.

## What the linter adds on top of the schema

The JSON Schema covers structure. [`modelith lint`](./07-cli.md) adds:

- **Semantic** checks, which split by severity:
  - **Errors** (broken references — the model can't be right):
    - a relationship target that doesn't reference a defined entity;
    - a relationship declared from both sides with cardinalities that aren't
      inverses (e.g. `Project`→`Policy` `1:n` but `Policy`→`Project` `1:1`);
    - a relationship declared from both sides where both ends claim
      `ownership: owned` — a relationship is owned by at most one end;
    - a duplicate invariant `id` (across entity-level *and* model-level
      invariants — they share one namespace);
    - a scenario `invariants_touched` or an action `preserves` that references an
      invariant id no entity or model-level invariant declares;
    - an [import](#imports) that is absolute, unreadable, not a domain model,
      binds a scope another import already bound, or is a bare path whose
      filename yields no valid slug;
    - a qualified attribute `type` whose scope isn't imported, or that names
      no enum in the model it resolves to.
  - **Warnings** (likely-but-not-certainly wrong):
    - a backticked term in freeform text that resolves to no entity, glossary
      term, role, or actor;
    - a relationship `role` that resolves to neither an entity nor a glossary
      term — define it in the glossary;
    - a relationship `role` that reads as prose (too long for a label, more
      than four words, or ending a sentence) — the role is the only label on
      the rendered diagram line, so the explanation belongs in `note`;
    - a pair where one end declares the same relationship more than once and
      the other declares it back, so which is the reciprocal of which can't be
      determined — the diagram draws every declaration as its own line;
    - an attribute `type` that looks like an enum reference (PascalCase) but
      names no defined enum — a *warning* where the qualified `scope.Name` form
      is an error, for the reason given under [Imports](#imports);
    - an action `actor` that is neither a defined entity nor a glossary term.
- **Completeness** checks (advisory warnings): entities with no invariants;
  entities no scenario exercises; a glossary term nothing references; an enum no
  attribute uses; an [import](#imports) nothing references.

  These are advisory on purpose. An entity that genuinely has no rule to state
  is fine — leave its invariants empty rather than inventing a filler rule that
  only restates its cardinality or its type. The warning is a prompt to check,
  not a demand to fill.
