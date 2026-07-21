# Architecture Decision Records

Forward-looking decision records. An ADR captures a decision or intention *as
it is made* — the direction chosen and why — so the plan stays current and a
future reader understands the constraint.

ADRs are numbered sequentially (`0001-slug.md`, `0002-slug.md`) and never
edited in place: a new ADR supersedes an old one. The shape, template, and the
bar for writing one live in
[`.claude/rules/adr.md`](../../.claude/rules/adr.md).

## ADRs vs. audits

Its sibling [`../audits/`](../audits/) is retrospective: point-in-time
snapshots of multi-agent reviews and their findings. When a decision comes out
of an audit, record the decision here and leave the audit as the evidence
trail.
