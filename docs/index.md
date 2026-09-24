---
sidebar_position: 1
title: Modelith - Domain Model Tooling
description: Author, validate, and render domain models by talking to an AI agent.
---

# Modelith - Domain Model Tooling

Modelith helps you author, validate, and render **domain models**: a
plain-language expression of a system's concepts, relationships, and governing
rules. Keep one model as the source of truth so everyone works from the same
picture.

The model lives as a YAML file, but **you rarely write that YAML by hand**. You
[author it by talking to an AI agent](./02-getting-started.md): you describe
concepts in plain language, the agent drafts and validates the YAML, and it
renders a Markdown version (with diagrams) that you commit alongside your code.
The `modelith` CLI is the engine the agent and CI run for you. It validates,
renders, and manages vendored models; it is not your starting point.

## The workflow

```
describe concepts in plain language
   │
   ▼
Claude Code skill (author) ─▶ writes / updates the YAML
   │
   ▼
model.modelith.yaml  ─▶ canonical source (you edit this, via the agent)
   │
   ├─▶ modelith lint    : validate + completeness  ─▶ CI gate
   └─▶ modelith render  : Markdown + Mermaid        ─▶ model.modelith.md committed to the repo
```

The rendered Markdown is **committed next to the YAML** so people and agents can
read the model without running anything. Configure `modelith render --check` in
CI to fail on drift, like a generated-code check. The GitHub Action enables that
check by default.

## Where to start

- **Authoring a model for the first time?** → [Getting Started](./02-getting-started.md)
  — install the CLI and authoring skills, then build a model by conversation.
- **Want to understand a model someone produced?** → [Understanding Your
  Model](./03-understanding-your-model.md) and [Reading the
  Diagrams](./04-reading-the-diagrams.md).
- **See it all come together?** → the [Parking Garage worked
  example](./05-parking-garage/index.md) builds a real model from nothing.

## The pieces

| Piece | What it does |
|---|---|
| [Agent authoring](./02-getting-started.md) | The Claude Code plugin and skills — how you actually build a model |
| [Schema](./06-schema-reference.md) | The JSON Schema that defines a valid model |
| [`modelith` CLI](./07-cli.md) | The `lint`, `render`, and `deps` commands the agent and CI run |
| [GitHub Action](./08-github-action.md) | The same checks in CI |
| [Vendoring](./10-vendoring.md) | Referencing a model that lives in another repository |
