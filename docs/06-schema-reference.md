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

## Relationship

| Field | Type | Required | Notes |
|---|---|---|---|
| `entity` | string | yes | Target entity name. Must reference a defined entity. |
| `cardinality` | string | yes | Written `left:right` (see below). `1:1`, `1:n`, `n:1`, `n:n` are the common shorthands. |
| `symmetric` | boolean | no | The relationship carries no inherent order: `(a, b)` is the same as `(b, a)`. Only valid on a self-referential relationship or one whose target side is more than one. |
| `role` | string | no | The role the related entity plays. Backtick entity names. |
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
  types are being explored
  ([issue #11](https://github.com/stacklok/modelith/issues/11)).
- **Domain events.** There is no event construct. A state change is expressed as
  an `action` plus the invariants it `preserves`, and enums **name** states
  while invariants govern the legal transitions between them. Deliberate, and
  consistent with why enums carry no transition edges.
- **Bounded contexts and context maps.** One model is one context. There is no
  construct for relating multiple contexts or mapping shared concepts across
  them. Compose several `*.modelith.yaml` models at the repository level instead
  of expressing context boundaries inside one file.

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
    - a duplicate invariant `id` (across entity-level *and* model-level
      invariants — they share one namespace);
    - a scenario `invariants_touched` or an action `preserves` that references an
      invariant id no entity or model-level invariant declares.
  - **Warnings** (likely-but-not-certainly wrong):
    - a backticked term in freeform text that resolves to no entity, glossary
      term, role, or actor;
    - a relationship `role` that resolves to neither an entity nor a glossary
      term — define it in the glossary;
    - an attribute `type` that looks like an enum reference (PascalCase) but
      names no defined enum;
    - an action `actor` that is neither a defined entity nor a glossary term.
- **Completeness** checks (advisory warnings): entities with no invariants;
  entities no scenario exercises; a glossary term nothing references; an enum no
  attribute uses.

  These are advisory on purpose. An entity that genuinely has no rule to state
  is fine — leave its invariants empty rather than inventing a filler rule that
  only restates its cardinality or its type. The warning is a prompt to check,
  not a demand to fill.
