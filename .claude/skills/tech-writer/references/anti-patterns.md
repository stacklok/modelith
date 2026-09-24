# Write-time anti-patterns

[Technical writing skill](../SKILL.md)

These are the habits that most often degrade documentation drafts. Read this list before drafting and again during self-check. Most of them are natural tendencies of LLM-generated prose, which is exactly why they need active resistance at write time rather than cleanup at review time.

Apply the same catalog when reviewing documentation. When a new recurring
pattern warrants repository guidance, update this file rather than creating a
second review-only copy.

## Proportionality

The deepest form of over-documentation isn't repetition; it's flat coverage. Fix it at the outline stage, before any prose exists.

**Flat option coverage.** A guide that gives every option, flag, and field equal billing reads like a control panel, not a recipe. The reader can't tell the paved road from the escape hatch, and the workflow most readers came for drowns in the long tail. Budget coverage by the share of readers who will actually use something: the common path gets the worked example and the narrative; a legitimate but situational option gets a sentence naming the situation and a link to reference; an esoteric knob gets nothing in the guide at all, because the reference already describes it and the readers who need it know to look. Two tests for any option you're about to document in a guide:

- If you were helping a colleague do this task at their desk, would you bring this option up unprompted? If not, it doesn't belong in the guide's main path.
- Does the option change what the reader _does_, or just what a value _is_? Changed workflows can earn guide coverage; values that exist belong in reference.

**Completeness reflex.** The instinct that a feature isn't fully documented until every capability appears in a guide somewhere. Completeness is reference's job, and the generated reference already provides it. A guide that covers less but is followable end to end serves more readers than one that covers everything. When an engineer's PR or spec lists ten configuration fields, that's an input inventory, not an outline; the outline comes from what readers are trying to do.

## Rhythm and punctuation

**Em dashes, and their disguises.** Never use `—` or `–`. Just as important: don't mechanically swap in a spaced hyphen or a comma while keeping the same "clause - punchy addendum" rhythm; that cadence reads as AI-generated even with the character fixed. Actually restructure: split the sentence, subordinate the clause, or cut the addendum. Spaced hyphens are only for list-style separators ("Topic - description" entries).

**Symmetric triads and balanced pairs.** "Fast, secure, and reliable." "Not just X, but Y." These rhetorical rhythms are filler in technical prose. If the three adjectives each carry a verifiable fact, keep the facts and lose the drumbeat; usually only one of them matters.

## Framing

**Changelog framing.** "Starting in v0.41", "previously", "now supports", "moved from X to Y". Docs state current behavior; release notes tell the story of change. This leaks in most when drafting from a PR or release diff: you're looking at a delta, but the reader needs a state. Write the page as if the behavior had always been this way. (Narrow exception for breaking changes: see "Document current behavior" in the style guide.)

**Negative restatement.** "Uses X, not Y." "Don't point it at the public endpoint." If the negation carries no new fact, cut it; if it carries one, fold that fact into the positive statement ("point it at the internal endpoint, which...").

**Buried lede.** The key fact ("this is automatic", "this requires Kubernetes") arrives after paragraphs of preamble. Lead every page and every section with the thing the reader most needs; the background can follow for those who keep reading.

**Dramatized limitations.** State gaps and limitations accurately and neutrally ("the UI doesn't yet expose this setting; use the CLI"), without apology, alarm, or spin in either direction.

**Marketing adverbs.** "Simply", "easily", "seamlessly", "powerful". If it's simple, the short instruction demonstrates that; saying so just mocks the reader for whom it isn't.

## Substance

**Hedging.** "May", "might", "could potentially", "should generally". Look up what actually happens and state it. If behavior is genuinely conditional, name the condition instead of hedging.

**Hedged lists.** "Clients such as VS Code and Cursor..." when the supported set is knowable. State the full list or link to the canonical reference (usually a compatibility page); "such as" invites the reader to guess.

**Over-explaining.** Restating what a command obviously does, defining concepts the audience (developers and DevOps professionals) already has, narrating the obvious consequence of the previous sentence. Trust the reader and cut.

**PR jargon leak.** "Consumers", "surface area", "wire up", "shapes". These come from the engineering artifact you're drafting from, not the reader's vocabulary. Name the actual components, fields, and values.

**Unverified facts.** Never write a flag name, field, default, or version constraint from memory. Verify against source, the schema, or the generated reference, and cite where it came from when presenting the draft.

**Placeholder examples.** `my-server`, `example-org`, `foo` where a real value exists. Use real values for fixed things (commands, image names, registry servers), `<ALL_CAPS>` for values the reader supplies, and reserved domains (`example.com`) in URLs, never real domains.

## Structure

**Listitis.** Bullets are for genuinely enumerable items. Consecutive single-sentence bullets that each begin with a bolded phrase are prose wearing a costume; readers can't follow an argument chopped into fragments. Write connected paragraphs for reasoning and comparisons (or a table when the items are truly parallel facts).

**Mirror-image sections.** "When to use X" followed by "When to use Y" that just inverts it. Write the comparison once, as a table or one honest paragraph of trade-offs.

**Admonition abuse.** An admonition must add information beyond the adjacent prose. If a note restates or negates what the paragraph just said, cut it. If it contains the only documentation of a feature, a worked example, or a full config block, it isn't a note; promote it to a section with a heading. One or two admonitions per page is the norm.

**Heading proliferation.** A heading per paragraph makes a page read like an outline and bloats the ToC. Sections need enough substance to deserve a name; merge stubs.

**Duplicated content.** Detailed content lives in exactly one place; other pages link to it with a line of context. If you're pasting a paragraph you wrote for another page, stop and link instead.
