# Audits

Durable records of multi-agent audits run against this repo. Each audit is a
point-in-time snapshot: which reviewers we ran, what they found, and how (or
whether) each finding was resolved. The goal is to make audits **repeatable**,
**learnable**, and **honest about state** — findings are never deleted, only
annotated with their resolution.

## Why this exists

A one-off audit is a sunk cost. By recording the agent roster, parameters, and
findings in a stable format we can:

- **Rerun** the same review later and diff the findings (did we regress? did we
  actually fix what we said we fixed?).
- **Learn** which reviewer lenses earned their keep and which produced noise or
  false alarms.
- **Onboard** contributors into how we reason about quality here — the prompts
  are the institutional knowledge.

## Layout

```
project-docs/audits/
├── README.md                          # this file — the process
├── TODO.md                            # prompts for scheduled audits
└── YYYY-MM-DD-<slug>.md               # one record per audit run
```

One file per run, date-prefixed so they sort chronologically. A rerun of a prior
audit should reference the earlier file and focus on the diff.

## The model

Each audit is a fan-out of independent **reviewer agents**, each with a narrow
charter (a domain lens + a specific file set), run in parallel, read-only. Tier
the model per agent: cheap models (Haiku) for lookup-against-a-rubric work, mid
(Sonnet) for config/code review with some reasoning, top (Opus) only where the
call is genuinely judgment-heavy. Recording the tier lets you check after the
fact whether cheaper agents missed things the expensive ones caught.

### Reviewer agents are read-only

Audit agents are instructed not to modify files. Fixes happen afterward, as
normal commits/PRs, and are recorded back into the audit file. This keeps the
"what we found" artifact separate from the "what we changed" history.

## How to run an audit

1. **Pick the roster.** Choose the domain lenses that fit the repo's surface
   (devops, security, language idiom, schema/format, docs-accuracy, domain
   modeling, CLI/UX, agent-prompt quality, …). Add or drop lenses to fit what
   actually exists. Don't run a lens with nothing to review.
2. **Tier the models.** Default Sonnet; drop mechanical lenses to Haiku; reserve
   Opus for judgment-heavy lenses (design trade-offs, what-to-omit calls).
3. **Write per-agent charters.** Each charter names the exact files to read and
   the specific questions to answer, and demands a prioritized list with
   `SEVERITY · location · issue · concrete fix`. The charters are recorded in the
   audit file so the run is reproducible.
4. **Launch in parallel, read-only.** Fan them out concurrently.
5. **Synthesize.** Cross-reference findings — items flagged by multiple agents
   rise to the top. Reconcile contradictions. **Flag false alarms** so nobody
   acts on bad input.
6. **Record.** Write the audit file using the template below.

## Triage (owner) — before fixing

Between recording the audit and starting fixes, the repo owner triages. This is
the human pass that the agents can't do: deciding what's worth fixing, in what
order, and what context the fixer needs.

Two mechanisms, both in the audit file:

- **Fix flags.** Each finding carries a `Fix` flag the synthesizer sets and the
  owner can override:
  - `easy` — mechanical, low-risk, no decision needed; safe to batch.
  - `Q` — has an open question; needs a quick decision/answer before fixing.
  - `design` — needs design work or discussion; bigger than a quick answer.
  - `—` — no action (Dismissed / Won't fix / Deferred).
- **Triage notes.** A dedicated section near the top of the audit file where the
  owner adds free-form notes keyed by finding ID *before* fixing — answers to
  `Q`s, ordering, "do this, skip that," extra context. These are the owner's
  input; the per-finding **Resolution** field (filled later) is the record of
  what was actually done. Keeping them separate preserves the order of events:
  decision → action.

Annotate by editing the `Fix` column and/or adding bullets to the triage section.
Don't edit the original finding text.

## How to record a fix

When a finding is resolved, **do not edit or delete the original finding text.**
Instead:

- Change the finding's **Status** (`Open` → `Fixed` / `Won't fix` / `Deferred`).
- Fill in **Resolution**: the commit/PR, the date, and a one-line "how" — what
  changed and why that closes it. For `Won't fix`, record the rationale.

This preserves the original observation alongside the decision, so the record
reads as a history, not a to-do list that erases its own past.

### Status values

| Status | Meaning |
| --- | --- |
| `Open` | Not yet addressed. |
| `Fixed` | Resolved in code/docs; Resolution names the commit/PR. |
| `Won't fix` | Deliberately not addressing; Resolution records why. |
| `Deferred` | Real, but parked behind a dependency or a design decision; Resolution notes what it's waiting on. |
| `Dismissed` | Not a real finding (false alarm / agent error); Resolution explains. |

## Finding IDs

Each finding gets a stable ID: an area prefix + number (`SEC-1`, `DDD-9`). IDs
are **never reused or renumbered** — they're how reruns and commits reference a
specific observation. Area prefixes: `OPS` (devops), `SEC` (security), `GO`
(Go code), `SCHEMA` (JSON Schema/format), `DOCS` (docs accuracy), `DDD` (domain
modeling), `CLI` (CLI/UX), `AGENT` (agent/plugin prompts).

## Record template

```markdown
# Audit: <title> — YYYY-MM-DD

- **Repo state:** <commit>, branch <branch>, <toolchain versions>
- **Scope:** <what was / wasn't reviewed>
- **Run by:** <human> via <N> parallel read-only reviewer agents

## Reviewer roster
| Lens | Model | Charter (files + focus) |
| ... |

## Priority rollup
<cross-cutting synthesis: what to do first, what multiple agents flagged,
contradictions reconciled, false alarms dismissed>

## Findings
### <AREA>
| ID | Sev | Fix | Finding | Status | Resolution |
| ... |

## Appendix: agent charters
<the prompts, for reproducibility>
```
