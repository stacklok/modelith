---
sidebar_position: 2
title: Getting Started
description: Install the CLI and the Claude Code plugin, then build your first domain model by talking to an agent.
---

# Getting Started

You should rarely write domain-model YAML by hand. The workflow is to describe
concepts, relationships, and scenarios in plain language to an AI agent and have
it construct and update the YAML. The YAML is the *output* of that conversation,
not the starting point. This page gets you set up and into that loop.

## Install

You need two things, in this order: the **CLI** (the engine that lints and
renders the YAML) and the **Claude Code plugin** (the skills that drive it by
conversation). The skills shell out to the `modelith` binary, so install it
first — otherwise your first skill invocation fails on a missing binary
instead of doing anything useful.

### 1. Install the CLI

```sh
brew install stacklok/tap/modelith
```

Or download a prebuilt binary, or build from source with `go install` — see
the full [CLI installation](./07-cli.md#installation) instructions for every
option.

### 2. Install the plugin

The skills ship as a **Claude Code plugin**. The plugin files live in
[`plugin/`](https://github.com/stacklok/modelith/tree/main/plugin), next to
the binary they drive, so the skills and CLI version stay in lockstep.

:::caution[Not yet published in the marketplace]

The `claude plugin marketplace` commands below are the intended install path, but
the plugin hasn't been submitted to `anthropics/claude-plugins-community` yet (see
the [roadmap](/)). **These commands will fail until the plugin is published.**

In the meantime, install from a local checkout:
[Developing the plugin locally](./09-local-development.md#developing-the-plugin-locally).

:::

Once the plugin is published, add the community marketplace once, then install:

```sh
claude plugin marketplace add anthropics/claude-plugins-community
claude plugin install modelith@claude-community
```

Restart Claude Code (plugin changes apply on the next session). The skills are
namespaced under the plugin name — invoke them as `/modelith:domain-model-author`,
`/modelith:domain-model-lint`, and `/modelith:domain-model-context`. You can also
browse and install interactively with the `/plugin` command.

Alternatively, install with the [skills CLI](https://skills.sh) (also works with
Cursor, Windsurf, and other agents):

```sh
npx skills add stacklok/modelith
```

<details>
<summary>If the install looks like it did nothing</summary>

A plugin install can occasionally wedge into a half-state — the UI shows the
plugin but flags it "not cached (not recorded)", and you can't enable or
uninstall it. Recover by clearing it and reinstalling:

```sh
claude plugin uninstall modelith@claude-community   # may fail; continue anyway
claude plugin marketplace update claude-community
claude plugin install modelith@claude-community
```

If `uninstall` errors with "not installed in user scope," remove
`"modelith@claude-community"` from `enabledPlugins` in `~/.claude/settings.json`
and delete `~/.claude/plugins/cache/claude-community/modelith`, then reinstall.

</details>

To develop or test the plugin from a local checkout instead of the marketplace,
see [Developing the plugin locally](./09-local-development.md#developing-the-plugin-locally).

## The skills

### `domain-model-author`

The primary skill. You describe a concept, relationship, or scenario; it:

- reads the schema and the existing model so new entries stay consistent,
- asks clarifying questions where a definition is fuzzy (the fuzziness is the
  signal — it means the model is incomplete there),
- drafts or updates the YAML following the backtick convention,
- runs `modelith lint` and resolves what it can,
- regenerates the committed Markdown.

### `domain-model-lint`

Runs `modelith lint` and explains the findings in context — which are hard errors,
which are advisory gaps, and what a good fix looks like. Use it as a focused
review pass.

### `domain-model-context`

Loads the rendered model into a coding session so the agent reasons in the same
terms the team uses — entity names, relationships, and invariants become a
"type system for the problem space."

## Build in passes

Don't fill in one entity completely before starting the next — build in passes
across the whole model. The author skill drives this for you, but it helps to
know the shape:

- **Pass 1:** name every entity, write each a crisp definition, and declare the
  relationships and cardinality between them. That alone renders to a real ER
  diagram and is the *minimum useful model* — worth circulating before you add
  detail.
- **Pass 2:** add the invariants and scenarios.
- **Pass 3:** fill in attributes, enums, actions, and glossary detail where they
  add clarity.

You can stop after any pass and have something honest.

## A worked session

A short, representative exchange with the `domain-model-author` skill — note that
the value is in the questions it asks, not the typing:

> **You:** A project can have policies attached to it.
>
> **Agent:** Before I add that — what *is* a policy here? A few questions so the
> model is sharp:
> 1. Does a `Policy` belong to exactly one `Project`, or can one policy be shared
>    across projects?
> 2. If a `Project` is deleted, do its policies go with it (`ownership: owned`) or
>    just get dereferenced (`referenced`)?
> 3. What must *always* be true of a policy? (That's the invariant.)
>
> **You:** One project only, and deleting the project deletes its policies. A
> policy is meaningless without its project.
>
> **Agent:** That's an owned, one-to-many relationship — a `Project` has many
> `Policies`, each part of exactly one. I'll declare it from the **parent** (the
> owning `Project` side, by convention) and add the `Policy` entity with its
> invariant:
>
> ```yaml
> Project:
>   relationships:
>     - entity: Policy
>       cardinality: "1:n"
>       ownership: owned
>
> Policy:
>   definition: >
>     A rule set evaluated by the system, belonging to exactly one `Project`. A
>     `Policy` has no meaning outside its owning `Project`.
>   invariants:
>     - id: belongs-to-one-project
>       statement: "Belongs to exactly one `Project`"
> ```
>
> Running `modelith lint`… clean. Regenerating `model.modelith.md`. One gap: no scenario
> exercises `Policy` yet — want to add one (e.g. "attach a policy to a project")?

The agent drafted YAML, validated it, regenerated the Markdown, and surfaced a
completeness gap as a question — the loop you'll repeat as the model grows.

For a full session start to finish — building a parking-garage model from
nothing across all three passes, including a moment where the agent catches a
contradiction in the requirements — see the [Worked Example: Parking
Garage](./05-parking-garage/index.md).

## Why this matters for coding agents

A well-formed model passed as context acts as a type system for the problem
space: the agent knows what entities exist, what they're called, how they
relate, and what invariants must hold. That produces more consistent code,
better names, and fewer wrong guesses. The more complete and accurate the model,
the less you have to correct.
