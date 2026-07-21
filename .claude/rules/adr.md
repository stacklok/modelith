---
paths:
  - "project-docs/adr/**/*.md"
description: ADR shape, template, when to write one, and how ADRs relate to audits
---

# ADR shape

ADRs live in `project-docs/adr/`, numbered sequentially: `0001-slug.md`,
`0002-slug.md`. Scan for the highest existing number and add one.

## ADRs vs. audits

Both live under `project-docs/`, and they are not the same thing:

- **`adr/` is forward-looking.** An ADR records a decision or intention *as it
  is made* — the direction chosen and why — so the plan stays current and a
  future reader understands the constraint. This is the home for "we decided
  X."
- **`audits/` is retrospective.** An audit is a point-in-time snapshot of a
  multi-agent review: which lenses ran, what they found, how each finding
  resolved. Findings are annotated, never deleted.

When a decision comes *out of* an audit, record the decision as an ADR and
leave the audit as the evidence trail. Don't relitigate a decision by editing
its ADR; supersede it.

## Template

```md
# {Short title of the decision}

{1-3 sentences: the context, what we decided, and why.}
```

An ADR can be a single paragraph. The value is recording *that* a decision was
made and *why*, not filling out sections. Add these only when they earn their
place:

- **Status** frontmatter (`proposed | accepted | superseded by ADR-NNNN`) —
  when decisions get revisited.
- **Considered options** — when a rejected alternative would otherwise be
  re-proposed in six months.
- **Consequences** — when downstream effects are non-obvious.

## When to write one

All three must be true:

1. **Hard to reverse** — changing your mind later costs something.
2. **Surprising without context** — a future reader would wonder why.
3. **A real trade-off** — genuine alternatives existed.

The format-evolution rules in `CLAUDE.md` ("stay on schema v1 while
pre-release", "no legacy string forms") are exactly the kind of decision an
ADR captures. As those are revisited, record the change as a new ADR.

## Calibration

- Length matches the decision's blast radius. Default: under 50 lines. Prose
  follows the same plain-writing bar as the published docs.
- A capability claim ("library X can do Y") lands with the measurement or
  spike output that proved it, inline or linked.
- A decision that a test can pin lands with a `TestADR_NNNN_<ShortName>` test
  in the same change (see [`testing.md`](testing.md)).
- Decisions are never edited in place; a new ADR supersedes the old one. Don't
  delete superseded ADRs.
