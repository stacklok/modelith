# Where the skills in this directory came from

Provenance and licence notes for skills imported from outside this repo. This
file is not a skill and is not loaded into an agent's context; it exists so the
origin of borrowed text is auditable.

**Convention:** when importing a skill from elsewhere, add an entry here
recording the upstream source, the commit it was taken at, its licence, and
what was changed. Skills written from scratch for modelith don't need an entry.

## grilling

- **Upstream:** [`mattpocock/skills`](https://github.com/mattpocock/skills),
  `skills/productivity/grilling/SKILL.md`
- **Taken at:** commit `ed37663cc5fbef691ddfecd080dff42f7e7e350d` (2026-07-21)
- **Licence:** MIT — Copyright (c) 2026 Matt Pocock. Full text:
  <https://github.com/mattpocock/skills/blob/main/LICENSE>
- **Changes:** none; the body is verbatim upstream.

## grill-with-docs

- **Upstream:** same repo, `skills/engineering/grill-with-docs/SKILL.md` and
  the `skills/engineering/domain-modeling/` skill it delegates to, at the same
  commit.
- **Licence:** MIT, as above.
- **Changes:** substantially rewritten for this repo. Upstream maintains a
  `CONTEXT.md` glossary and its own ADR format doc; both were dropped, because
  modelith's schema, structs, and schema reference already hold that vocabulary
  under test, and `.claude/rules/adr.md` already sets the ADR bar. The retained
  upstream material is the session-conduct advice (sharpen fuzzy language,
  stress-test with scenarios, cross-reference the code) plus the
  emission-frequency probe.

Upstream also ships an installer (`npx skills@latest add mattpocock/skills`,
from [`vercel-labs/skills`](https://github.com/vercel-labs/skills)) and a
Claude Code plugin marketplace entry. Neither is used here: the installer
writes no lockfile, so it records no less than this file does, and both of the
skills above are edited rather than tracked upstream.
