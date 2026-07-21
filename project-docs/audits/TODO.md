# Pre-open-source audit

Run this audit immediately before making the repo public. The goal is to confirm
there are no internal references, security issues, or docs accuracy problems that
would embarrass the project or expose Stacklok internals.

## Scope

Whole repo at the commit that will become the initial public commit. Excluded:
runtime/perf benchmarks, end-to-end consumer-repo integration.

## Reviewer roster

Use the `/code-review` skill or fan out agents manually. Suggested lenses:

| Lens | Model | Charter |
| --- | --- | --- |
| **Internal-ref sweep** | Haiku | Every file in `docs/`, `plugin/`, `README.md`, `CLAUDE.md`, `audits/`, `action.yml`, `Taskfile.yml`, `.github/workflows/`, `.goreleaser.yaml`. Hunt for: any mention of internal tooling, private repos, Stacklok-internal URLs, GOPRIVATE, proprietary language, NDA language, pre-release product names or codenames, internal Slack channels or runbooks, anything that assumes the reader is a Stacklok employee. Report every hit with file + line. Include `project-docs/`. |
| **License / legal** | Haiku | Every file. Confirm `LICENSE` is Apache 2.0. Check for any file-level copyright headers that still say "All rights reserved" or reference proprietary licensing. Check `go.mod` dependencies for license-compatibility problems (GPL, etc.). |
| **CLI / DX** | Haiku | `cmd/modelith/`, `action.yml`, `README.md`. Command/flag coherence, exit codes, help text, output streams, README-vs-impl drift. Mechanical rubric check: does every documented flag exist? does every existing flag appear in `--help`? do exit codes match documented behavior? |
| **DevOps / GH** | Sonnet | `.github/workflows/*.yml`, `action.yml`, `.goreleaser.yaml`, `Taskfile.yml`, `go.mod`. CI coverage and caching, action versions and Node compatibility, build-from-source design, release correctness, `go.mod` dependency hygiene. Apply public-project standards — no "internal only" dismissals. |
| **JSON Schema** | Sonnet | `internal/schema/v1/modelith.schema.json`, `internal/schema/schema.go`, `examples/`. Draft-2020-12 correctness and strictness, version/kind evolution strategy (is the current approach sound for a public format?), required-ness gaps, self-consistency, and whether the schema teaches the format well to a first-time reader. |
| **Security** | Sonnet | `action.yml`, `.github/workflows/*.yml`, `.goreleaser.yaml`, `cmd/`, `internal/`. Public-repo threat model: supply-chain pinning, shell injection, secret scope, GITHUB_TOKEN permissions, any path/input that accepts user-controlled data and passes it to a shell or filesystem unsanitized. No "internal only" dismissals. |
| **Docs accuracy** | Sonnet | `README.md`, `docs/**`, `plugin/skills/**`. Claims-vs-reality is the #1 job: every install command, every flag, every URL, every roadmap checkbox. Flag anything a first-time external user would find broken or misleading. Check that all `modelith.sh` URLs are structurally correct (path matches the numbered doc files). Check that `anthropics/claude-plugins-community` and `npx skills add` install paths are accurate given the actual plugin manifest. |
| **Go code** | Sonnet | `cmd/`, `internal/**`. Idiom, package boundaries, error handling, test coverage gaps. No `Won't fix / internal tool` dismissals — apply public-project standards. |
| **DDD (pragmatic)** | Opus | `internal/schema/v1/modelith.schema.json`, `internal/lint/`, `examples/`, `docs/06-schema-reference.md`. Is the DDD-lite vocabulary coherent and sufficient for an external audience? What is worth adding vs. deliberately omitting? Where are the modeling foot-guns for a new user? Does the parking-garage example teach the format well, and does it exercise the important cases? |
| **Plugin / skills** | Opus | `plugin/**` vs. `internal/schema/` and the CLI. Skill triggering accuracy and overlap, CLI-instruction drift (do the skills reference the right binary name, flags, and file conventions?), whether the author skill reliably produces lint-clean valid YAML, manifest correctness, guardrails for common mistakes, any skill body that still assumes Stacklok-internal context. |

## What to do with findings

Record this audit in a new `YYYY-MM-DD-pre-oss-audit.md` file using the template
in `project-docs/audits/README.md`. Triage before fixing: mark each finding `easy`, `Q`,
`design`, or `—`. Fix everything `easy` before opening the repo. Anything `Q`
needs a decision first. Park `design` items as issues.

Do not open the repo until the internal-ref sweep and license/legal lenses come
back clean.
