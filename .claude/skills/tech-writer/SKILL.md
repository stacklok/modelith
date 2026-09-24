---
name: tech-writer
description: >
  Use when writing or substantively editing user-facing documentation: drafting a new page, rewriting or restructuring an existing one, adding a major section, or turning engineering material (PR descriptions, specs, release notes, rough notes) into docs. Writes clear, focused technical documentation following the Diataxis framework and modelith's local documentation conventions. Use it even when the request doesn't mention writing quality; it governs how documentation gets written. Not for editorial review of finished work.
---

# Technical writing

Write documentation as a senior technical writer: clear, accurate, and focused on what the reader needs to accomplish. Every page has one primary reader need, and the discipline of this skill is deciding which one before writing a word, then keeping that purpose clear. Brief supporting context from another mode is often useful; it should help the reader without competing with the page's primary purpose.

Everything in this skill and its references is guidelines, not rules. Each one explains its reasoning so you can depart from it when doing so genuinely improves the content for the reader, knowing why you're departing. What's never optional is the judgment itself: a rule followed into an absurd result is as much a failure as a rule ignored.

## Canonical sources

Don't duplicate guidance; read it from where it lives:

1. **The local [style guide](references/style-guide.md)** provides prose and
   style guidance.
2. **[`docs/_docs-conventions.md`](../../../docs/_docs-conventions.md)** owns
   public-documentation placement, links, and verification for modelith.
3. **`CLAUDE.md` and `.claude/rules/`** own contributor-documentation and
   repository-process guidance.
4. **The mode references** in [`references/`](references/) provide Diataxis
   discipline and write-time anti-patterns.

## Workflow

1. **Classify.** Use the compass below to decide the page's primary mode. Include brief in-situ context from another mode when it helps the reader understand or complete the task. Split supporting material into a separate page only when it warrants a full discussion or workflow, or when it would compete with the page's primary purpose. Keep the modes distinguishable without creating a separate page for every type of content.
2. **Place.** For public documentation, follow
   [`docs/_docs-conventions.md`](../../../docs/_docs-conventions.md). For
   contributor documentation, follow `CLAUDE.md` and the applicable
   `.claude/rules/` guidance. Update the owning page; do not append the same
   feature narrative to several pages. Create a page only for a distinct reader
   need.
3. **Read.** Read the reference file for your mode, plus [the write-time anti-patterns](references/anti-patterns.md), plus the style guide sections your task touches. For a new page, also skim 1-2 existing pages of the same type in the same section so the new page reads like a sibling, not a transplant.
4. **Draft.** Outline first, weighting coverage by real-world use: the workflow most readers came for gets the worked example and the narrative; situational options get a sentence and a reference link; esoteric knobs stay in reference (see "Proportionality" in the anti-patterns file). Then write for the reader described in the mode reference, stating the most important thing first on the page and in each section.
5. **Self-check.** Before presenting the draft, reread it against the anti-patterns file and the mode's "keep out" list. Cut what fails. For substantial new content, use an independent editorial review when available; for small edits, the self-check is enough.

## The compass: classifying content

Two questions determine the mode: does the content inform the reader's _action_ (doing) or _cognition_ (understanding), and does it serve the _acquisition_ of skill (learning) or the _application_ of skill (working)?

| Content...        | ...serves skill... | Mode        | It is...     |
| ----------------- | ------------------ | ----------- | ------------ |
| informs action    | acquisition        | tutorial    | a lesson     |
| informs action    | application        | how-to      | a recipe     |
| informs cognition | application        | reference   | a map        |
| informs cognition | acquisition        | explanation | a discussion |

A quick tiebreaker: ask what the reader is doing when they open the page. Learning by following along means tutorial. Getting a real task done means how-to guide. Looking something up means reference. Trying to understand why or how something works means explanation.

## The four modes

- **Tutorial** - a guided lesson where you take responsibility for the reader's success. Quickstarts and end-to-end getting-started pages. Read [the tutorial guidance](references/tutorials.md).
- **How-to guide** - a recipe for a competent user with a real task. Usually the bulk of a documentation set: task-oriented guides and integration walkthroughs. Read [the how-to guidance](references/how-to-guides.md).
- **Reference** - neutral, complete description of the machinery: CLI commands, API and schema specs, configuration fields, compatibility tables. Often auto-generated; check the project's rules before touching generated files, since fixes usually belong upstream. Read [the reference guidance](references/reference.md).
- **Explanation** - understanding-oriented discussion of concepts, background, and design reasoning. Concept pages and product introductions. Read [the explanation guidance](references/explanation.md).

## Reference files

| When you are...                                | Read                          |
| ---------------------------------------------- | ----------------------------- |
| Writing or editing a tutorial or quickstart    | [Tutorials](references/tutorials.md)             |
| Writing or editing a how-to guide              | [How-to guides](references/how-to-guides.md)      |
| Writing or editing reference material          | [Reference](references/reference.md)              |
| Writing or editing concept/explanation content | [Explanation](references/explanation.md)          |
| Drafting anything (always, before self-check)  | [Write-time anti-patterns](references/anti-patterns.md) |
| Checking style, structure, or terminology      | [Style guide](references/style-guide.md)           |

## Self-check

Before presenting a draft, verify:

- [ ] The page has a clear primary mode. Supporting context from another mode helps that purpose; material that warrants a full discussion or competing workflow was split out and linked.
- [ ] The most important point leads the page and each section; no buried ledes.
- [ ] Coverage is proportional to real-world use: the common workflow carries the page, situational options get a sentence and a reference link, and nothing is documented just because it exists.
- [ ] Every factual claim about behavior, flags, fields, or defaults was verified against source, specs, or generated reference docs, not recalled from memory. Living docs describe implemented behavior, not merely an approved plan.
- [ ] Outdated and duplicate text was replaced or deleted. Only unique, verified knowledge was migrated; implementation chronology stays in PRs/Git rather than a catch-all notes page.
- [ ] Code examples work as written: real values for fixed things, `<ALL_CAPS>` placeholders for reader-supplied values, reserved domains (`example.com`) in URLs.
- [ ] The draft passes the anti-patterns file: no changelog framing, negative restatement, redundant admonitions, hedging, listitis, or em-dash rhythm.
- [ ] Front matter (where the site uses it) has `title` and a `description` whose first 70 characters stand alone.
- [ ] How-to guides and tutorials end with the project's closing-section pattern (for Stacklok docs: Next steps, then Related information, then Troubleshooting, in that order, as applicable).
- [ ] The page is reachable: a navigation/sidebar entry plus inbound links from related pages.
- [ ] Terminology matches the style guide's word list.
