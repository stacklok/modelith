# Writing reference material

[Technical writing skill](../SKILL.md)

Reference is a map. The reader is working and needs to look something up: a flag, a field, a default, a supported version. They consult reference material the way they consult a dictionary; nobody reads it front to back. Its entire value is that the reader can trust it and find things in it fast.

Reference material typically covers CLI commands, API and schema specs, configuration fields, and compatibility tables, either in a dedicated reference section or as reference pages inside product sections.

**Check what's auto-generated first.** CLI reference pages, CRD reference pages, and API specs are often generated from upstream sources; check the project's conventions (CLAUDE.md or a docs README usually lists the paths and rules). Never hand-edit generated files: fixes go upstream, and hand-written framing usually has a designated home the generator preserves. This reference file applies to hand-written reference pages and to reference sections within other pages.

## Principles

**Describe, and do nothing but describe.** State what exists, what it does, its type, its default, its constraints. Instruction, persuasion, opinion, and explanation all belong elsewhere. The single job is accurate description.

**Be austere and neutral.** Plain, factual statements in consistent patterns. Reference material is where dry writing is a virtue: the reader is scanning, and flourishes slow them down. Say each thing once, the same way you said the analogous thing about the neighboring field.

**Mirror the structure of the machinery.** Organize reference the way the thing itself is organized: fields in the order the schema defines them, subcommands under their parent command, one section per resource. The reader navigates the docs with their mental map of the product; the two should match.

**Be complete and accurate above all.** A reference that omits a field or describes a stale default is worse than none, because readers stop trusting all of it. Verify every value against the current source or schema, never memory. When the product changes, the reference changes; state only current behavior.

**Consistency beats elegance.** Identical things get identical treatment: same column order in every table, same sentence pattern for every field description, same placeholder conventions. Predictability is what makes reference scannable.

**Examples illustrate; they don't teach.** A short example showing a field's valid values or a typical stanza is good reference. A worked scenario with narrative is a how-to guide leaking in; move it.

## Keep out of reference

- Step-by-step instructions (link to the how-to guide instead)
- Explanations of why the design is the way it is (link to concept pages)
- Recommendations and opinions ("we suggest...", "the best option is...")
- Marketing language; reference has no adjectives to sell
- Changelog framing; describe the current version of the machinery

## Structure

Reference structure follows the shape of the thing described, so there is no fixed skeleton. Common patterns:

- Tables for enumerable facts (clients, versions, fields, defaults) with explanation kept to prose around the table, not crammed into cells
- One heading per command, resource, or field group, in the product's own order
- Front matter `title` and `description` like every page; a "Related information" closing section linking to the guides that use this machinery
