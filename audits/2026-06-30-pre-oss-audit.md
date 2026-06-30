# Pre-OSS Audit — 2026-06-30

**Status:** 🚫 2 blockers — DO NOT OPEN until resolved

## Summary

| Lens | Findings | Blockers | Easy | Q | Design |
|------|----------|----------|------|---|--------|
| Internal-ref sweep | 5 | 0 | 5 | 0 | 0 |
| License / legal | 0 | 0 | 0 | 0 | 0 |
| CLI / DX | 0 | 0 | 0 | 0 | 0 |
| DevOps / GH | 9 | 0 | 6 | 2 | 0 |
| JSON Schema | 9 | 0 | 9 | 0 | 0 |
| Security | 4 | 0 | 1 | 3 | 0 |
| Docs accuracy | 4 | 2 | 4 | 0 | 0 |
| Go code | 6 | 0 | 4 | 0 | 0 |
| DDD (pragmatic) | 7 | 0 | 0 | 0 | 7 |
| Plugin / skills | 4 | 0 | 2 | 2 | 0 |
| **Total** | **48** | **2** | **31** | **7** | **7** |

License/legal and CLI/DX came back **clean**.

---

## Blockers (fix before opening)

- **[docs-accuracy]** `README.md:37` · easy · Plugin install commands advertise a marketplace entry that isn't published yet
  > README lines 37-39 present `claude plugin marketplace add anthropics/claude-plugins-community` + `claude plugin install modelith@claude-community` as working install steps. The roadmap at line 97 explicitly marks 'Published to anthropics/claude-plugins-community' as unchecked (TODO). Anyone following these instructions will get a 'not found' error. Either gate this doc section behind a "coming soon" admonition, or replace with the `--plugin-dir` local-checkout path as the interim primary path until the plugin is actually published.

- **[docs-accuracy]** `docs/02-getting-started.md:22` · easy · Same phantom marketplace install commands in the primary user-facing doc
  > docs/02-getting-started.md lines 22-24 show the claude-community install as the primary install path for a first-time user with no caveat that it doesn't work yet. The recovery block at lines 49-57 even describes uninstall/reinstall flows that presuppose a working install. Until the plugin is published, prefix with a clear "coming soon" note or replace with the `--plugin-dir` local-checkout path from docs/09-local-development.md.

---

## High

- **[internal-ref]** `plugin/.claude-plugin/plugin.json:4` · easy · Plugin description refers to 'Stacklok domain models' which brands the tool as company-specific
  > Change to: 'Author, validate, and render domain models...'

- **[internal-ref]** `plugin/skills/domain-model-lint/SKILL.md:4` · easy · Skill description refers to 'Stacklok domain model' instead of generic 'domain model'
  > Change: 'Run modelith lint on a Stacklok domain model...' → 'Run modelith lint on a domain model...'

- **[internal-ref]** `plugin/skills/domain-model-context/SKILL.md:4` · easy · Skill description refers to 'Stacklok domain model' instead of generic 'domain model'
  > Change: 'Load a Stacklok domain model...' → 'Load a domain model...'

- **[devops]** `.github/workflows/deploy-docs.yml:24` · easy · All four actions use floating mutable tags, not SHA pins
  > actions/checkout@v4, actions/setup-node@v4, actions/upload-pages-artifact@v3, actions/deploy-pages@v4 — all mutable tags. ci.yml and release.yml already demonstrate the correct SHA+comment pattern. Pin all four before opening.

- **[devops]** `.goreleaser.yaml:1` · easy · No SBOM generation or artifact signing configured
  > Add `sboms` block and Sigstore keyless signing (`cosign` block) to goreleaser. Also add `id-token: write` to the release workflow for OIDC. Public OSS users can't verify provenance without this.

- **[security]** `.github/workflows/deploy-docs.yml:24` · easy · (same as devops finding above — high-impact because this workflow has `id-token: write` and `pages: write`)

- **[DDD]** `docs/06-schema-reference.md:79` · design · Silent on the four DDD concepts an external practitioner will look for first
  > The schema explains why state transitions are absent, but never addresses aggregates, value objects, domain events, or bounded contexts. A DDD-literate reader reads the silence as an oversight. Add a "What this format deliberately leaves out" section to the schema reference. Highest-leverage doc change for credibility with the target audience.

---

## Medium

- **[internal-ref]** `plugin/.claude-plugin/plugin.json:6` · easy · Author field lists 'Stacklok' — change to 'Modelith contributors' for a public OSS project

- **[devops]** `.goreleaser.yaml:14` · Q · Windows absent from build targets — action.yml will fail on Windows runners. Decide: add Windows support, or explicitly document it as unsupported.

- **[devops]** `.github/workflows/ci.yml:26` · easy · `go test` runs without `-race`; add to both CI and Taskfile

- **[devops]** `.github/workflows/release.yml:8` · easy · `contents: write` at workflow level — move to job level for tighter scoping

- **[schema]** `internal/schema/v1/modelith.schema.json:193` · easy · `attribute` `else` branch error message is cryptic ('value is not valid' against schema `false`) — add `$comment` to surface 'derivation is only allowed when derived is true'

- **[schema]** `internal/schema/v1/modelith.schema.json:121` · easy · `entity.attributes` array missing description unlike all other entity sub-arrays (relationships, actions, invariants all have one)

- **[security]** `.goreleaser.yaml:6` · Q · Pre-hook `go mod tidy` at release time can silently alter go.sum against what CI tested. Remove or move the tidy check to CI.

- **[docs]** `docs/06-schema-reference.md:9` · easy · Schema URL rendered as clickable hyperlink but not yet live. Move the caveat above the link, or render as plain text until the schema is served.

---

## Low / Info

- **[internal-ref]** `plugin/.claude-plugin/plugin.json:9` · easy · Keywords array includes 'stacklok' — remove it

- **[devops]** `.goreleaser.yaml:23` · easy · Archives are tar.gz only — if/when Windows is added, use `format_overrides` to produce zip

- **[devops]** `.github/workflows/ci.yml:1` · Q · No golangci-lint — vet+staticcheck is solid but golangci-lint is the de facto public Go standard. Intentional skip should be documented.

- **[devops]** `go.mod:7` · easy · Deps slightly stale (patch-level): jsonschema v6.0.1→v6.0.2, yaml v1.4→v1.6, pflag v1.0.9→v1.0.10, x/text. Run `go get -u ./... && go mod tidy` before first release.

- **[devops]** `action.yml:27` · — · No explicit "caller must run actions/checkout first" in description or inputs. Low priority.

- **[schema]** `internal/schema/v1/modelith.schema.json:148` · easy · `cardinality` and `ownership` enum fields lack `type: string` — reduces editor autocomplete quality

- **[schema]** `internal/schema/v1/modelith.schema.json:156` · easy · `ownership` field should declare `default: "referenced"` to match description and docs

- **[schema]** `internal/schema/v1/modelith.schema.json:66` · easy · No `title` on any `$defs` entry — editor hover shows raw `$ref` path fragments instead of concept names

- **[schema]** `internal/schema/v1/modelith.schema.json:204` · easy · `action` $def has no title/description; `actionObject` has no description — oneOf union is under-documented for editors

- **[schema]** `internal/schema/v1/modelith.schema.json:258` · easy · `scenario` has no `description` property (unlike `actionObject`). Mark Q — intentional omission or gap?

- **[schema]** `internal/schema/v1/modelith.schema.json:1` · easy · No root-level `examples` array — editors use this for snippet generation

- **[schema]** `docs/06-schema-reference.md:32` · easy · `entities` table says required=no but doesn't note the `minProperties: 1` constraint. `entities: {}` will fail schema validation with a confusing message.

- **[security]** `.github/workflows/release.yml:8` · Q · `contents: write` at workflow level — scope to job level (overlaps devops finding)

- **[security]** `.github/workflows/ci.yml:18` · Q · Go toolchain floating (`stable`) in CI and release — pin to `1.26.x` in CI/release; keep `stable` default in action.yml but document the trade-off

- **[docs]** `README.md:93` · easy · Roadmap notes "Scenario sequenceDiagram rendering (Markdown-only today)" but docs never mention this limitation in-context. Add one sentence to 03-understanding-your-model or 06-schema-reference.

- **[go]** `internal/lint/lint.go:171` · easy · Schema recompiled on every `lint.Run` call — use `sync.Once` in schema package to cache the compiled schema

- **[go]** `cmd/modelith/main.go:128` · easy · `os.ReadFile` error returned bare (inconsistent with adjacent `lint.Run` error which wraps with path). Same in renderCmd:206.

- **[go]** `cmd/modelith/main_test.go:1` · easy · No CLI-level test for `--completeness=error` promoting completeness warnings to blocking exit

- **[go]** `cmd/modelith/main_test.go:1` · easy · No test for `render --check` when committed .md file doesn't yet exist

- **[go]** `internal/schema/schema.go:52` · — · `var URL` is set-once but declared as var — document as immutable; not actionable until package is public API

- **[go]** `internal/lint/lint.go:73` · — · Package-level compiled regexp and printer vars — acceptable for internal CLI, note for completeness

- **[plugin]** `plugin/skills/domain-model-author/SKILL.md:105` · easy · Skill instructs agent to write the `$schema` header but never gives the literal URL — agent may omit or fabricate it. State the exact URL or say "copy the `$id` from `modelith schema` output."

- **[plugin]** `plugin/skills/domain-model-lint/SKILL.md:19` · easy · Not-installed recovery is vague ("see CLI docs") — mirror the author skill's concrete `go install` command

---

## Questions (need a decision)

| # | File | Question |
|---|------|----------|
| Q1 | `.goreleaser.yaml:14` | Support Windows? Or explicitly document as unsupported in action.yml + README? |
| Q2 | `.github/workflows/ci.yml:1` | Add golangci-lint, or document the intentional skip? |
| Q3 | `.goreleaser.yaml:6` | Remove `go mod tidy` pre-hook, or add a tidy-check to CI instead? |
| Q4 | `.github/workflows/release.yml:8` | Move `contents: write` to job level? (Easy if yes.) |
| Q5 | `.github/workflows/ci.yml:18` | Pin Go toolchain version in CI/release? |
| Q6 | `plugin/skills/domain-model-author/SKILL.md:36` | Confirm modelith.sh docs are live before launch, or soften the link? |
| Q7 | `internal/schema/v1/modelith.schema.json:258` | Add `description` field to `scenario`? Intentional omission or gap? |

---

## Design items (park as GitHub issues)

1. **DDD omissions undocumented** — `docs/06-schema-reference.md:79` · Add "What this format deliberately leaves out" section covering aggregates, value objects, domain events, bounded contexts with a per-item out-of-scope/roadmap call.

2. **Glossary/entity namespace collision unguarded** — `internal/schema/v1/modelith.schema.json:28` · A glossary key that promotes to an entity is silently ambiguous. Add a lint error (cheap, next to the existing duplicate-invariant-id check).

3. **Completeness check pressures junk invariants** — `internal/lint/lint.go:496` · The "entity has no invariants" warning + strict CI + author skill teaches newcomers to invent cardinality-restating filler. Soften the message; document that genuinely rule-free entities are fine.

4. **Ticket as value object in flagship example** — `docs/05-parking-garage/garage.modelith.yaml:222` · Ticket is a textbook value object modeled as an owned 1:1 entity. The worked example should name the tension explicitly, turning a hidden limitation into a teaching moment.

5. **Cardinality optionality foot-gun** — `internal/schema/v1/modelith.schema.json:148` · 1:n carries no optionality; minimums live as invariants. The Mermaid renderer outputs `||--o{` (zero-or-many) even when an invariant says "at least one." Document in schema-reference and check 04-reading-the-diagrams.

6. **Example violates its own relationship guidance** — `docs/06-schema-reference.md:113` · The reference says prefer single-side declaration, but example.modelith.yaml declares Project↔User n:n from both ends redundantly. Fix example or make this the explicit "both ends add clarity" case with explanation.

7. **Scenario steps have no stress-test convention** — `docs/06-schema-reference.md:188` · The reference calls scenarios "diagnostics" but nothing enforces or teaches the violation-then-refusal pattern. Document the convention (at least one scenario per invariant that attempts to violate it).
