---
name: domain-model-context
description: >
  Load a domain model into the working context before a coding task.
  Use at the start of a session that will touch product concepts, when the user
  says "use the domain model" or "load the model," or before implementing a
  feature in a repo that has a *.modelith.yaml. Makes the agent reason in the
  team's vocabulary — entity names, relationships, and invariants.
---

# Loading the domain model as context

A well-formed domain model is a type system for the problem space. Loading it
before a coding task means you use the team's exact names, respect the declared
relationships, and don't violate invariants.

## What to load

1. Find the domain model in the repo — a `*.modelith.yaml` and its rendered
   `*.modelith.md` (often under `docs/` or the repo root).
2. **Prefer the rendered Markdown** for reading: it's the readable form, with
   the relationship diagram inline. If it's missing or stale (check with `modelith
   render --check <file>` — the file argument is required), regenerate it with
   `modelith render <file>` first. (If `modelith` isn't installed, you can still read the
   committed `.md` directly; just note it may be stale.)
3. Read it in full before writing code. Internalize:
   - the **canonical names** — use `Project`, never "workspace" or "container";
   - the **relationships and cardinality** — what owns what;
   - the **invariants** — rules your code must not break.

## How to apply it while coding

- Name variables, types, functions, and UI strings using the model's terms.
- When a requirement seems to need a concept the model doesn't have, stop: that
  may be a real gap. Flag it and offer to capture it with the
  `domain-model-author` skill rather than silently inventing a name.
- When code would violate an invariant, treat that as a bug in the plan, not a
  detail to smooth over.

## Keep the model honest

If implementing the feature reveals the model is wrong or incomplete — a missing
entity, an invariant that can't hold — surface it. The model is meant to be a
living source of truth; coding against it is exactly when its gaps show up.
