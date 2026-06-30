# Modelith

> **Early days.** The schema, CLI, output, and docs may still change in breaking
> ways. Feedback is very much appreciated — kick the tires and file issues.

Tooling for authoring, validating, and rendering **domain models** — the
canonical, plain-language expression of what a system *is*: its concepts, how
they relate, and the rules that govern them.

A domain model lives as a YAML file, but **you rarely write that YAML by hand** —
you author it by talking to an AI agent. The agent drafts and validates the YAML
and renders it to Markdown (with embedded Mermaid diagrams); you commit both
alongside your code. The `modelith` CLI is the engine the agent and CI run for you.

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

CI regenerates the Markdown and fails on drift (`modelith render --check`) — like
a generated-code check.

## Getting started

You need two things, in this order: the **CLI** (the engine that lints and
renders) and the **Claude Code plugin** (the skills that drive it by
conversation) — the skills shell out to the binary, so install it first.

**1. Install the CLI:**

```sh
brew install stacklok/tap/modelith
```

Or download a prebuilt binary from the
[Releases page](https://github.com/stacklok/modelith/releases), or build from
source:

```sh
go install github.com/stacklok/modelith/cmd/modelith@latest
```

**2. Install the Claude Code plugin** (the skills that author the model):

> **Marketplace listing pending review.** The plugin has been submitted to
> `anthropics/claude-plugins-community` and is awaiting approval — **the
> commands below will fail until it's listed.** In the meantime, use the
> skills CLI below, which works today.

Install with the [skills CLI](https://skills.sh) (also works with Cursor, Windsurf, and others):

```sh
npx skills add stacklok/modelith
```

Once the marketplace listing is approved:

```sh
claude plugin marketplace add anthropics/claude-plugins-community
claude plugin install modelith@claude-community
```

**3. Author by conversation.** In Claude Code, invoke `/modelith:domain-model-author`
and describe your domain — the agent asks the questions, drafts the YAML, lints
it, and keeps the rendered Markdown in sync.

→ Full walkthrough: **[Getting Started](https://modelith.sh/getting-started)**.

## The format, briefly

A `*.modelith.yaml` file is **self-describing** (`kind` + `version`) with four
top-level sections — `glossary`, `enums`, `entities`, and `scenarios`:

```yaml
# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container owned by at least one `User`.
    invariants:
      - id: at-least-one-owner
        statement: "Must have at least one `Owner` at all times"
```

## Documentation

Full docs: **[modelith.sh](https://modelith.sh)**

- [Getting Started](https://modelith.sh/getting-started) — install the plugin, author by conversation
- [Understanding Your Model](https://modelith.sh/understanding-your-model) — what the agent produces
- [Worked Example: Parking Garage](https://modelith.sh/parking-garage/) — a full session, start to finish
- [Schema Reference](https://modelith.sh/schema-reference) · [CLI](https://modelith.sh/cli) · [GitHub Action](https://modelith.sh/github-action)

Working **on** modelith itself? See [CLAUDE.md](CLAUDE.md) for repo layout, the
dev workflow, and the conventions to keep.

## Roadmap

- [x] Schema, `lint`, `render` (Markdown + Mermaid `erDiagram`, with `--check`)
- [x] GitHub Action + GoReleaser + CI
- [x] Claude Code plugin + skills (author / lint / context)
- [x] Docs at [modelith.sh](https://modelith.sh)
- [ ] Published to `anthropics/claude-plugins-community` (submitted, pending review)
- [ ] Serve the schema at `modelith.sh` (editor autocomplete)
- [ ] Scenario `sequenceDiagram` rendering (Markdown-only today)

## License

Apache 2.0 — see [LICENSE](./LICENSE).
