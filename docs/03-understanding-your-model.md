---
sidebar_position: 3
title: Understanding Your Model
description: "What the agent produces: the YAML's core concepts, backtick convention, and two files you commit."
---

# Understanding Your Model

You author by conversation, but you still own the result — and you should be
able to read it without the agent. For a model your repository owns, commit two
files:

- **`model.modelith.yaml`** — the canonical source. Self-describing (`kind` +
  `version`), it's what the agent edits and what CI validates.
- **`model.modelith.md`** — the rendered Markdown (with an embedded Mermaid
  diagram). This is the easiest-to-read form; people and agents read it
  directly, and CI fails if it drifts from the YAML.

Commit both. The Markdown is generated from the YAML — never hand-edit it; change
the YAML (via the agent) and re-render. A model [vendored from another
repository](./10-vendoring.md) is a copy with different rendering obligations.

## Model contents

A model can define `glossary`, `enums`, `entities`, `scenarios`, and model-level
`invariants`:

- **`glossary`** — ubiquitous-language terms that aren't entities (roles like
  `Owner`, states, domain nouns), each with a definition.
- **`enums`** — first-class enumerated types, referenced by an attribute's
  `type`.
- **`entities`** — the named concepts, each with a definition, relationships,
  attributes, actions, and invariants (each invariant carries a stable `id`).
- **`scenarios`** — short narratives that exercise the entities to stress-test
  whether the model hangs together. Scenarios render as formatted text steps
  today; `sequenceDiagram` rendering is a roadmap item.

Model-level `invariants` hold rules that span several entities and have no
natural single owner (for example, "when a `Project` is archived, none of its
`Policies` remain enabled"). They use the same `{id, statement}` shape as entity
invariants and share their ID namespace.

A model can also declare `imports` to reference enums and entities defined by
another model. Imports use files already in your repository. If the other model
originates elsewhere, [vendoring](./10-vendoring.md) copies it into your
repository; it remains a vendored model with different rendering obligations.
See the [Schema Reference](./06-schema-reference.md) for these and all other
top-level fields.

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
linter can check they reference real entities. Freeform text renders as
Markdown; raw HTML is rendered as literal text. In structured fields that
already imply an entity (`actors`, relationship `entity:`, entity keys), the
backticks are skipped. The agent follows this automatically; it's worth
recognizing when you read the YAML.
