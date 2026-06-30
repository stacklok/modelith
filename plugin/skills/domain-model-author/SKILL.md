---
name: domain-model-author
description: >
  Build or update a domain model by conversation. Use when the user wants to add
  or change an entity, relationship, attribute, action, invariant, or scenario in
  a *.modelith.yaml file, or describes product concepts they want captured in the
  model. Drafts the YAML, asks clarifying questions where a definition is fuzzy,
  runs the linter, and regenerates the committed Markdown. For a read-only check
  that explains findings without editing, use domain-model-lint instead.
---

# Authoring a domain model

A domain model is the canonical, plain-language expression of what a product
*is*: its named concepts (`entities`), how they relate, the rules that govern
them (`invariants`), and short narratives that stress-test the whole
(`scenarios`). The YAML is the output of a conversation, not something the user
hand-writes. Your job is to run that conversation and produce well-formed YAML.

## Before you write anything

1. **Check that `modelith` is installed.** Run `modelith --version` (or `command -v modelith`).
   If it is not on `PATH`, **stop** and tell the user to install it:
   `go install github.com/stacklok/modelith/cmd/modelith@latest`, or download a
   release binary from https://github.com/stacklok/modelith/releases. Do **not**
   hand-write YAML you cannot lint — the linter is what keeps the model valid;
   without it you are guessing.
2. **Read the schema.** Run `modelith schema` — that is the source of truth and works
   wherever `modelith` is installed. (Only when you are working *inside the
   modelith repo itself* may you instead read
   `internal/schema/v1/modelith.schema.json` directly; that path does not exist
   in a consuming repo.) Every file you produce must validate against it.
3. **Read the existing model.** Find the relevant `*.modelith.yaml` in the repo
   and read it in full, plus its rendered `.md` if present. New entries must be
   consistent with existing names and relationships.
4. **Read the philosophy** if you have not: the [modelith docs](https://modelith.sh)
   explain *why* naming and invariants matter.

## Build order — skeleton first

A model isn't built field-by-field down one entity; it's built in **passes
across the whole model**, each pass a coherent layer you could stop at. Going
deep on one entity — all its attributes, actions, enums — before the others
exist is the fastest way to a lopsided model and an exhausted user. Work
breadth-first, in this order:

1. **Pass 1 — the skeleton (do this completely first).** Name every entity and
   give each a crisp two-to-four-sentence `definition`; declare the
   `relationships` and `cardinality` between them. Nothing else yet. This is the
   highest-leverage work in the whole exercise — naming is the commitment — and
   it already renders to a real ER diagram. **This is the minimum useful
   model**: a skeleton that lints and reads as a coherent artifact is worth
   circulating before any detail is added. Don't move past Pass 1 until the
   whole skeleton stands.
2. **Pass 2 — the behavior.** Add `invariants` (the rules that must *always*
   hold) and `scenarios` (short narratives that exercise every entity). These
   need the skeleton first — you can't write a sharp invariant before the nouns
   and structure are settled. The linter's completeness layer nudges here: it
   warns about entities with no invariants and entities no scenario touches.
   Attach an invariant to the entity it governs; if a rule genuinely spans
   several entities with no single owner, put it in the top-level `invariants`
   list instead (see "Writing the YAML").
3. **Pass 3 — refinement.** Fill in `attributes` (and `derived` ones), `enums`,
   `actions`, `glossary` roles, `ownership`, and `role`/`note` detail — but only
   where they add clarity. This is a long tail, not a checklist to exhaust. The
   "defined but never referenced" warnings on glossary and enums exist precisely
   to stop you front-loading detail that hasn't earned its place.

You can stop after any pass and have something honest. Treat Passes 2 and 3 as
enrichment of a model that is already useful, not as the work of "finishing" it.

## The conversation

Your value is in the questions, not the typing. The questions track the passes
above — settle naming, relationships, and cardinality across the whole model
before you probe rules and scenarios. Probe until the model is sharp:

- **Naming is a commitment.** When the user says "rule" or "config," ask what
  the concept actually *is*. The right name (`Policy` vs `Rule` vs `Constraint`)
  reflects a decision about what it does and what it includes/excludes.
- **Fuzziness is a signal.** If the user can't give a crisp two-to-four-sentence
  definition, or isn't sure whether two things are the same concept, that gap is
  the point. Surface it; don't paper over it.
- **Push on invariants.** "What must *always* be true?" is where the real
  behavior lives. Write them short, declarative, and testable.
- **Ownership and cardinality.** For each relationship: one-to-one,
  one-to-many, or many-to-many? And is the related entity *part of* this one —
  can it exist on its own? `ownership: owned` means no: it's a part that can't
  exist without this entity (composition), so it's deleted along with it.
  `referenced` (or omitting `ownership`) means it's an independent entity this
  one merely points at. Anchor on "part-of / can't exist independently," not on
  vaguer notions of control or exclusivity.
- **Which side to declare from.** A relationship can be declared from either
  entity, and the linter lets you declare it from both (it checks the two
  cardinalities are inverses). Prefer declaring it once, from the **parent** —
  the entity that owns or contains the other, or sits on the "one" side of a
  one-to-many (declare `Project → Policy`, not the reverse). Use the parent's
  perspective whenever there's an intuitive one; reach for a both-sides
  declaration only when each view genuinely adds clarity.

## Writing the YAML

Follow the format exactly (see the schema reference). Key conventions:

- Top of file: the schema header (copy the exact `$id` from `modelith schema`
  output, or use `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json`),
  then `kind: DomainModel` and `version: v1`.
- Entity keys are canonical **PascalCase** names (`Project`, not `project`).
- **Backtick entity names in freeform text** — definitions, `role`, `note`,
  invariants, scenario steps: `` `Project` ``. In structured fields that already
  imply an entity (`actors`, relationship `entity:`, entity keys), do **not**
  backtick.
- `cardinality` is one of `1:1`, `1:n`, `n:1`, `n:n`. If you declare the same
  relationship from both entities, the two cardinalities must be inverses
  (`1:n` ⇄ `n:1`; `1:1`/`n:n` symmetric) — a contradiction is a lint **error**.
  Declaring it from one side only is fine and usually clearer.
- **`glossary`** (top level) defines non-entity vocabulary — roles like `Owner`,
  states, domain nouns — as `Term: "definition"`. Define any role or actor name
  here; an undefined role warns, and an unused glossary term warns.
- **`enums`** (top level) defines enumerated types by PascalCase name, each with
  `values` (a list of `{name, definition}`). An attribute selects one via
  `type: ProjectStatus`. Do **not** write `type: "enum[active, archived]"` — that
  is not a valid type; use the PascalCase name of an enum you defined under
  `enums`.
- **Attribute `type`** is a primitive in **lowercase** (`string`, `integer`,
  `boolean`, `timestamp`) or the **PascalCase name of a defined enum**. A
  PascalCase type that names no enum warns. Mark computed values with
  `derived: true` plus a `derivation:` (required when derived).
- **`actions`** items are either a bare string (`create`) or a structured object
  `{name, actor?, preserves?, description?}`. Use the object form to tie an
  action to its `actor` (an entity or glossary term) and the invariant ids it
  `preserves`. Mixed bare + structured in one list is fine.
- **`invariants`** items are `{id, statement}`. The `id` is lowercase
  kebab-case and unique across the model; `statement` is the rule (backtick
  entity names). References point at the **id**, never the text. Invariants live
  in one of two places: under the entity they govern (`entities.<X>.invariants`),
  or — for a rule that spans several entities with no single owner — in the
  **top-level `invariants`** list (sibling to `entities`). Both share the same
  shape and one id namespace. Prefer the entity scope; reach for the top-level
  list only when no single entity is the rule's home.
- Scenario `invariants_touched` is a list of invariant **ids**. Each must
  reference a declared invariant — a dangling id is a lint **error**. A reference
  resolves against both scopes (entity-level and top-level). If a scenario needs
  an invariant that doesn't exist yet, add it (to the owning entity, or the
  top-level list if cross-entity) and reference that.

### Hard rules the schema enforces (these are errors)

Get these right the first time; they fail `modelith lint` structurally:

- **No unknown keys, anywhere** (`additionalProperties: false` at every level). A
  misspelled field like `definiton:` is a hard error, not a silent no-op.
- **Required fields:** every entity needs a `definition`; every relationship
  needs `entity` + `cardinality`; every scenario needs `name` + at least one
  `steps` entry; every attribute needs `name` + `type`; every invariant needs
  `id` + `statement`; every enum needs at least one `values` entry; a `derived`
  attribute needs a `derivation`.
- **Entity keys and relationship targets are PascalCase**, two or more
  characters, no underscores or hyphens (`TargetUser`, not `target_user`).
- **`cardinality`** is exactly one of `1:1`, `1:n`, `n:1`, `n:n`; **`ownership`**,
  when present, is exactly `owned` or `referenced`.

### Avoid these linter false positives

The semantic checks are heuristic; these patterns trip warnings that aren't real
bugs, so steer around them:

- **Don't backtick a PascalCase word that isn't a defined entity, role, or
  actor.** ``a `Webhook` fires`` warns if `Webhook` is no entity — either define
  it, name it as an actor/role, or drop the backticks.
- **Irregular plurals aren't understood.** The linter matches `Policy`/`Policies`
  and `Project`/`Projects` (a naive pluralizer), but exotic plurals may not
  resolve — prefer the singular entity name in backticks.
- **`invariants_touched` and action `preserves` reference invariant ids**, not
  statements. Use the invariant's `id` (entity-level or top-level — both
  resolve); a non-existent id is an error (not a silent miss), so it's safe — but
  make sure the id actually exists.
- **An action `actor` must be a defined entity or glossary term.** `actor: admin`
  (lowercase, or any name you haven't declared) warns — define it in the
  `glossary` first, or use the entity name.

## Always finish by validating and rendering

After any edit:

```sh
modelith lint <file>           # resolve errors; explain remaining warnings to the user
modelith render <file>         # regenerate the committed Markdown
```

**Reading exit codes:** `modelith lint` exits non-zero when there are errors (or, with
`--completeness error`, on completeness gaps) — that is the model telling you to
fix something, not a tool failure. A usage error is different: an `error:` line
about a missing file or unknown flag is a bad invocation, not a finding.
Distinguish the two before reacting, and don't loop on a usage error.

Keep the `.yaml` and the regenerated `.md` in sync, and never edit the `.md` by
hand — it is generated. Commit them **together when the user is ready**; don't
commit on your own unless they ask.

## Changing vs. adding

- **Adding** (new entity, attribute, scenario) is low-ceremony: draft, lint,
  done.
- **Changing** an existing definition, name, or cardinality is a breaking change
  that ripples through code, docs, and anything primed on the old meaning
  (including agents). Call this out explicitly, summarize the blast radius, and
  make sure the user wants it before proceeding.
