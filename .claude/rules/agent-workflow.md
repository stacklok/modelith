# Agent workflow: subagents, the review loop, and model discipline

The protocol for non-trivial changes. `CLAUDE.md` points here; read this
before starting a qualifying change or spawning agents for one.

This is available discipline for risky changes, not a mandate on every commit.
A one-line doc tweak or a mechanical rename doesn't need it. The bar is
judgment about what a change puts at risk, not a line count.

This protocol applies only in the top-level conversation with the user. A
delegated subagent implements its task itself. It never re-delegates the work
this document describes. The one exception is a subagent explicitly spawned as
a phase orchestrator (see below); its task prompt says so and points here.

## The review loop

A change that touches correctness-critical surfaces, or runs beyond roughly a
few hundred lines, goes through this loop:

1. Launch a subagent (`Agent` tool, `isolation: "worktree"`) to implement the
   change on its own branch.
2. Run `/code-review` against the resulting diff.
3. Fix what the review finds via a separate fix subagent, not by hand-editing
   the result inline.
4. Repeat review then fix for up to three rounds. Record each round's findings
   and fixes in `.scratch/reviews/<change>.md`. When a change flows through a
   draft PR, the record goes to a PR comment instead. Stop early when a round
   comes back clean or leaves only non-blocking nits. Stop at three rounds
   regardless.
5. If the cap is hit with real issues still open, say so plainly in the final
   round's record. Never quietly declare the change done.

## Draft until vetted

When a change flows through a pull request, open it as a draft as soon as the
branch is first pushed, and keep it a draft while the review loop runs (its
round records go to PR comments, per step 4 above). Flip it to ready with
`gh pr ready` as soon as the review loop has cleared it with no blocking
findings. A ready pull request means the review loop vetted it; a draft means
the loop is still in progress.

Do not wait for CI before flipping, and do not poll for CI to finish. `main` is
branch-protected: the required checks (`build-test` and the DCO `check`) must be
green before a merge lands, so a ready pull request cannot merge on a red build.
Flipping to ready right after the review loop clears is enough — branch
protection holds the merge until CI passes.

A phase orchestrator applies this itself. It returns the pull request after
flipping it to ready. If it stopped with blockers still open, it leaves the
draft open and says so plainly.

## Scale round one to the change

The full fan-out is not the default. Match the first round's depth to what the
change puts at risk:

- **Full round** (`/code-review high`, all finder angles): changes touching
  correctness-critical surfaces — the schema and its sync with the model
  structs, the version registry and dispatch, the lint rules (structural,
  semantic, completeness), the Markdown and Mermaid renderers and their
  deterministic output, or the release and Action plumbing. Also for diffs
  beyond a few hundred lines, new mechanisms, or new dependencies.
- **Reduced round** (`/code-review medium`): everything else. The cleanup
  angles (reuse, simplification, efficiency) earn their tokens only in a full
  round; `/simplify` covers them on demand.
- **Docs-only diffs**: one reviewer pass. For a schema-reference or format doc
  that pass includes a lifecycle walkthrough: take every format concept the
  doc introduces and walk it through authoring, linting, and rendering from a
  user's seat. Hunt for claims that drift from what the CLI actually does.

At either code tier, add an adversarial runner alongside the review: the real
`modelith` binary against hostile fixtures (malformed YAML, missing invariant
references, dangling enum types, completeness gaps), comparing base against the
change. For a format tool it is consistently the highest-yield finder.

Attribute every finding in the round record to the angle or agent that sourced
it.

## Evidence standards

A correctness finding is CONFIRMED only when it carries an executable repro: a
fixture `*.modelith.yaml` plus the `modelith lint`/`render` command someone
else can run. Without one it is at most PLAUSIBLE and gets verified, not
trusted. Apply the same skepticism to verifier verdicts in both directions. A
REFUTED without quoted, checkable evidence refutes nothing. Spot-check by
running the repro, not by re-reading the argument.

Judgment calls hit during implementation or review follow the repo default:
ask early, don't guess. Switch to "proceed and record open questions" only
when told to for that specific task.

## Phase orchestration

Only the user can clear or compact the top-level session, so the main
conversation cannot shrink itself. It can only avoid growing. The mechanism is
delegating a whole phase, not just its pieces:

- When told not to wait on the user, wrap the entire implement-review-fix cycle
  in one phase-orchestrator subagent. It spawns its own implementer, finders,
  and fix agents and applies this document itself. It returns only the branch
  or PR, per-round outcomes in a few lines each, and any open questions it
  recorded. Its accumulated context is discarded when it completes instead of
  landing in the top-level session.
- In the default ask-early mode, keep the loop at top level: a nested
  orchestrator cannot surface questions mid-task, so delegating the phase
  silently converts ask-early into don't-wait. Contain context growth with
  `HANDOFF.md` instead. Refresh it at every phase boundary; when a boundary
  lands, tell the user it is a good point to clear the session.

## Subagent models

Token cost is a first-class constraint, not a tiebreaker. Pick each subagent's
model deliberately: the cheapest that works. Use the smallest model for narrow
mechanical lookups, a mid model for routine multi-step work, and the strongest
models only where reasoning depth demonstrably pays (correctness-critical
finder angles, verifiers that must construct repros, implementation with subtle
cross-file semantics such as a schema/struct change). State each spawn's model
and a one-line rationale when launching it. Later review rounds shrink:
delta-only scope, fewer finders, cheaper models first.

**Resuming a completed subagent is safe on Claude Code 2.1.211 or later.** On
earlier versions the resume silently drops the agent's spawn-time model
override and runs it on the session's current default model
(anthropics/claude-code#68147). On those versions (check `claude --version`)
spawn a fresh agent at the intended model for each later round, pointed at the
standing records (round files, draft PR, fix agenda) so re-derivation stays
cheap. Messaging an agent whose task is still active joins the run on its spawn
model on every version, so mid-task steering is always safe.

To audit what a spawn actually ran, do not trust self-reports. Grep the task
output JSONL:

```sh
grep -o '"model":"[^"]*"' <task-id>.output | sort | uniq -c
```

## The orchestrator's context budget

The orchestrating conversation is the most expensive context in play. Keep it a
thin router:

- Cap agent reports at roughly 200 words. Full evidence (adversarial matrices,
  per-finding detail, fixture inventories) goes to a file in `.scratch/`, a
  file committed in the agent's own worktree, or a PR comment, and the report
  names where it went.
- When spawning several agents against the same target, write the shared
  context once to a scratch brief. Each spawn prompt is a few lines: the
  brief's path plus that agent's specific angle.
- Do not pull large diffs or file dumps into the main thread when only a
  subagent needs them.
- Mechanical work (small greps, file moves, comment posting) runs inline with
  no subagent, but its bulky output still goes to files.

## See also

- [`../../project-docs/audits/README.md`](../../project-docs/audits/README.md)
  — the retrospective multi-agent audit process, a heavier relative of this
  loop.
