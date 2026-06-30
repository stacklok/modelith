---
sidebar_position: 3
title: Understanding Your Model
description: What the agent produces — the YAML's four sections, the backtick convention, and the two files you commit.
---

# Understanding Your Model

You author by conversation, but you still own the result — and you should be
able to read it without the agent. Every model produces **two committed files**:

- **`model.modelith.yaml`** — the canonical source. Self-describing (`kind` +
  `version`), it's what the agent edits and what CI validates.
- **`model.modelith.md`** — the rendered Markdown (with an embedded Mermaid
  diagram). This is the easiest-to-read form; people and agents read it
  directly, and CI fails if it drifts from the YAML.

Commit both. The Markdown is generated from the YAML — never hand-edit it; change
the YAML (via the agent) and re-render.

## The four sections

A `*.modelith.yaml` file has four top-level sections:

- **`glossary`** — ubiquitous-language terms that aren't entities (roles like
  `Owner`, states, domain nouns), each with a definition.
- **`enums`** — first-class enumerated types, referenced by an attribute's
  `type`.
- **`entities`** — the named concepts, each with a definition, relationships,
  attributes, actions, and invariants (each invariant carries a stable `id`).
- **`scenarios`** — short narratives that exercise the entities to stress-test
  whether the model hangs together. Scenarios render as formatted text steps
  today; `sequenceDiagram` rendering is a roadmap item.

A fifth, optional top-level section — **`invariants`** — holds rules that span
several entities and have no natural single owner (e.g. "when a `Project` is
archived, none of its `Policies` remain enabled"). It uses the same
`{id, statement}` shape as entity invariants and shares their id namespace; reach
for it only when a rule genuinely has no single home (see the
[Schema Reference](./06-schema-reference.md#invariant)).

A minimal model looks like this:

```yaml
# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
title: My Product

glossary:
  Owner: "A `User` with full control of a `Project`."
  Member: "A `User` with access to a `Project` but no ownership rights."

entities:
  Project:
    definition: >
      A container owned by at least one `User`.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: "`Owner` or `Member`"
    invariants:
      - id: at-least-one-owner
        statement: "Must have at least one `Owner` at all times"
  User:
    definition: A human principal who owns or belongs to `Projects`.
    invariants:
      - id: unique-email
        statement: "Email address is unique across all `Users`"

scenarios:
  - name: Create a project
    actors: [User]
    steps:
      - "A `User` creates a `Project` and becomes its `Owner`"
    invariants_touched: [at-least-one-owner]
```

See the [Schema Reference](./06-schema-reference.md) for every field, and [Reading
the Diagrams](./04-reading-the-diagrams.md) for how the rendered ER diagram works.

## The backtick convention

In freeform text (definitions, steps, invariants), entity names are wrapped in
backticks — `` `Project` `` — so the renderer formats them as code and the
linter can check they reference real entities. In structured fields that already
imply an entity (`actors`, relationship `entity:`, entity keys), the backticks
are skipped. The agent follows this automatically; it's worth recognizing when
you read the YAML.
