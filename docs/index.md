---
sidebar_position: 1
title: Modelith - Domain Model Tooling
description: Author, validate, and render domain models by talking to an AI agent.
---

# Modelith - Domain Model Tooling

Tooling for authoring, validating, and rendering **domain models** — the
canonical, plain-language expression of what a system *is*: its concepts, how
they relate, and the rules that govern them. One model, one source of truth:
everyone reads from the same picture.

The model lives as a YAML file, but **you rarely write that YAML by hand**. You
[author it by talking to an AI agent](./02-getting-started.md): you describe
concepts in plain language, the agent drafts and validates the YAML, and it
renders a Markdown version (with diagrams) that you commit alongside your code.
The `modelith` CLI is the engine the agent and CI run for you — it's there to
validate and render, not to be your starting point.

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

The rendered Markdown is **committed back next to the YAML** so people and
agents read the model directly, without running anything. CI regenerates it and
fails on drift (`modelith render --check`) — like a generated-code check.

## Where to start

- **Authoring a model for the first time?** → [Getting Started](./02-getting-started.md)
  — install the plugin, then build a model by conversation.
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
| [`modelith` CLI](./07-cli.md) | The `lint` / `render` engine the agent and CI run |
| [GitHub Action](./08-github-action.md) | The same checks in CI |
