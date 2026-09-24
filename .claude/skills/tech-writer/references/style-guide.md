# modelith documentation style guide

[Technical writing skill](../SKILL.md)

This guide is adapted from Stacklok's [shared tech-writer skill](https://github.com/stacklok/claude-plugins/tree/main/plugins/docs/skills/tech-writer) and kept in this repository so every contributor and coding harness can apply the same writing guidance. For modelith documentation ownership, links, and verification, follow [`docs/_docs-conventions.md`](../../../../docs/_docs-conventions.md).

## Content framework

We follow the [Diátaxis framework](https://diataxis.fr/#) of documentation structure, which defines four distinct types of documentation: tutorials, how-to guides, reference, and explanation.

This guide does not seek to reproduce the definitions and background found on the Diátaxis site. The best starting point for background is [Applying Diátaxis](https://diataxis.fr/application/). The tech-writer skill's mode references cover how each type is written.

## Writing style

This list is not exhaustive, it is intended to reflect the most common and important style elements. For a more comprehensive guide that aligns with our style goals, or if you need more details about any of these points, refer to the [Google developer documentation style guide](https://developers.google.com/style).

### Language

The official language is **US English**.

Avoid slang and colloquial expressions. Use clear, straightforward language and avoid overly complex jargon to make content accessible to a wide audience.

Translate engineering shorthand into concrete, reader-facing terms. Language from PR descriptions and commit messages ("consumers", "shapes", "surface area") rarely belongs in docs; name the actual components, fields, and values instead.

### Tone and voice

Strive for a casual and conversational tone without becoming overly informal. We aim to be friendly and relatable while retaining credibility and professionalism, approachable yet polished.

#### Active voice

Use **active voice** instead of passive voice. Active voice emphasizes the subject performing the action, making the writing more direct and engaging. Passive voice focuses on the recipient of the action rather than the actor, often resulting in unclear sentences and misinterpretation of responsibility.

:white_check_mark: Yes: Click **Install** to install the MCP server.\
:x: No: MCP server is installed when the "Install" button is clicked.

:white_check_mark: Yes: Set the `debug` flag to `true` to enable verbose logging.\
:x: No: Verbose logging is enabled when the `debug` flag is set to `true`.

#### Speak to the reader

Address the reader using the **second person** ("you", "your"). Avoid the first person ("we", "our") and third person ("the user", "a developer") in task-oriented guidance.

First-person plural is appropriate in explicitly labeled roadmap or direction content when it communicates project intent, for example, "We are working toward." Do not use it to lead readers through instructions.

### Document current behavior

Docs describe how the product works today; they are not a changelog or upgrade guide.

Don't narrate history or transitions. Avoid "Starting in vX.Y", "previously", "changed from X to Y", "new in this release", and version-conditional notes. Release notes and blog posts tell that story; guides and references state only the current behavior. Upgrade sequencing and migration caveats likewise belong in release notes, not inline in how-to guides.

:white_check_mark: Yes: The JSON-RPC error code for rate limiting is `429`.\
:x: No: The JSON-RPC error code moved from `-32029` to `429` in v0.41.0.

Prefer positive statements. Say what the product does and what the reader should do; don't restate a positive statement in negative form. If a negation carries a genuinely new fact, fold that fact into the positive statement.

:white_check_mark: Yes: ToolHive uses the registry policy's `server_api_url` value.\
:x: No: ToolHive reads `server_api_url`, not `api_url`.

**Exception for breaking changes**: a breaking change or major behavioral change (a changed default, behavior that silently differs for existing setups) may carry a clearly labeled, versioned admonition (e.g. `:::info[Changed in v0.30.1]`) when upgraders need an explanation or action they can't infer from an error message. Keep the surrounding prose standalone about current behavior; the admonition carries only the upgrade delta and action. Remove these notes after a few releases, once the upgrade audience has moved on. Changes that fail loudly at startup with an obvious cause don't qualify.

Large migrations (API version promotions, multi-field removals) get a dedicated migration guide; guide pages link to it with a short pointer rather than carrying the details inline.

Deprecation notices are current state, not changelog. Documenting that a feature is deprecated, still read but warned about, or scheduled for removal is fine.

### Capitalization

Capitalize **proper nouns** like names, companies, and products. Generally, **don't** capitalize features or generic terms. For non-Stacklok terms, follow the norms of the third-party project/company (ex: npm is stylized in lowercase, even when it begins a sentence).

:white_check_mark: Yes: Organize MCP servers into groups\
:x: No: Organize MCP Servers into Groups

Use **sentence case** in titles and headings.

:white_check_mark: Yes: Configuration file structure\
:x: No: Configuration File Structure

Use `<ALL_CAPS>` to indicate placeholder text/parameters, where the reader is expected to change a value.

### Punctuation

**Oxford comma**: use the Oxford comma (aka serial commas) when listing items in a series.

:white_check_mark: Yes: ToolHive requires minimal CPU, memory, and disk space.\
:x: No: ToolHive requires minimal CPU, memory and disk space.

**Quotation marks**: use straight double quotes and apostrophes, not "fancy quotes" or "smart quotes" (the default in document editors like Word/Docs). This is especially important in code examples where smart quotes often cause syntax errors.

**Dashes**: avoid em dashes (`—`) and en dashes (`–`). They are hard to type, easy to miss in editors, and have proliferated with AI-generated content. Instead, rephrase naturally: use commas, split into two sentences, or restructure. If a separator is truly needed (for example, between a link and its description in a list), use a spaced hyphen (`-`).

Tip: if you are drafting in Google Docs, disable the "Use smart quotes" setting in the Tools → Preferences menu to avoid inadvertently copying smart quotes into Markdown or other code.

### Links

Use descriptive link text. Besides providing clear context to the reader, this improves accessibility for screen readers.

:white_check_mark: Yes: For more information, see [Tone and voice](#tone-and-voice).\
:x: No: For more information, see [this section](#tone-and-voice).

Note on capitalization: when referencing other docs/headings by title, use sentence case so the reference matches the corresponding title or heading.

### Formatting

**Bold**: use when referring to UI elements; prefer bold over quotes. For example: Click **Install server** to start the installation process.

**Italics**: emphasize particular words or phrases, such as when introducing/defining a term. For example: Custom permissions are defined using _permission profiles_.

**Underscore**: do not use; reserved for links.

**Code**: use a `monospaced font` for inline code or commands, code blocks, user input, filenames, method/class names, and console output.

## Screenshots and images

Considerations for screenshots and other images:

- Don't over-use screenshots:
  - Screenshots are useful for complex UIs or to point out specific elements that are otherwise hard to describe with text. But for example, an input form doesn't need a screenshot when text can just as easily list the fields and their purpose.
  - Screenshots age rapidly.
  - Too many screenshots can become visually overwhelming and interrupt the flow of documentation.
- Don't use images of text, code samples, or terminal output. Use actual text so readers can copy/paste and find the contents via search engines.
- Use alt text to describe images for readers using screen readers and to assist search engines.
- Be consistent when taking screenshots - use the same OS if possible (macOS has been used in Stacklok docs to date) and zoom level (ex: zoom twice in VS Code, 125% in browsers).
- Crop screenshots to the relevant portion of the interface.
- Use the primary brand colors (`#2D684B` on light backgrounds, `#BDDFC2` on dark backgrounds) for annotations like callouts and highlight boxes.

## Page structure

Every how-to guide and tutorial page follows a consistent structure. This ensures readers always know what to expect and never hit dead ends.

### Front matter

On sites that use front matter (Docusaurus and most static site generators), every page must have front matter with at least a `title` and `description`.

#### Descriptions

The `description` field serves double duty: it appears in index/preview cards (truncated at roughly 70-75 characters) and as the page's `<meta>` description for search engines (ideally 80-150 characters total).

Write descriptions to work at both lengths:

- **Front-load the value.** The first 70 characters must stand alone as a useful summary, because that's all a preview card shows before cutting off.
- **Add SEO detail after the natural break.** Use the remaining characters for keywords and context that help search engines.
- **Lead with the action or topic, not filler.** Avoid openers like "Learn how to," "Understanding," "A guide to," or "This page describes."
- **Avoid special YAML characters in unquoted values.** Colons (`:`) inside a description can break YAML parsing. Either rephrase, use a comma, or wrap the value in quotes.

Examples:

:white_check_mark: `Install the ToolHive CLI and run your first MCP server in minutes.`\
:x: `A step-by-step guide to installing the ToolHive CLI and running your first MCP server.`

:white_check_mark: `Groups organize MCP servers into logical sets and control which clients can access them.`\
:x: `Understanding when and why to use groups for organizing MCP servers and controlling client access.`

### Closing sections

Every how-to guide and tutorial page ends with closing sections in this order:

1. `## Next steps` (required) - 1-3 links to the next logical pages, following the reader's journey (for example: install, use, secure, operate, optimize).
2. `## Related information` (optional) - links to background reading, reference docs, or external resources that don't represent a next action.
3. `## Troubleshooting` (optional) - common issues and solutions, typically using collapsible `<details>` blocks.

Example:

```mdx
## Next steps

- [Run MCP servers](./run-mcp-servers.mdx) to deploy your first server.
- [Client configuration](./client-configuration.mdx) to connect your IDE.

## Related information

- [Understanding MCP](../concepts/mcp.mdx) for background on the protocol.

## Troubleshooting

<details>
<summary>Server fails to start</summary>

Check that the container runtime is running...

</details>
```

### Introduction pages

Each major product or component section starts with an Introduction page that explains what the component is, who it's for, and where to start. Make it an explicit navigation entry, not a hidden category-link page.

### Cross-references

Link to related content in other sections where it adds value. When a page's topic connects to a concept page, a reference page, or a peer product's docs, link to it rather than restating the content. When products or components have overlapping names (for example, a built-in registry versus a registry server product), disambiguate at the point of reference.

## Markdown style

Just like a consistent writing style is critical to clarity and messaging, consistent formatting and syntax are needed to ensure the maintainability of Markdown-based documentation.

We generally adopt the [Google Markdown style guide](https://google.github.io/styleguide/docguide/style.html), which is well-aligned with default settings in formatting tools like Prettier.

Our preferred style elements include:

- Headings: use "ATX-style" headings (hash marks - `#` for Heading 1, `##` for Heading 2, and so on); use unique headings within a document
- Unordered lists: use hyphens (`-`), not asterisks (`*`)
- Ordered lists: use lazy numbering (`1.` for every item and let Markdown render the final order. This is more maintainable when inserting new items)
  - Note: this is a "soft" recommendation. It is also intended only for Markdown documents that are read through a rendering engine. If the Markdown will be consumed in raw form, use real numbering.
- Code blocks: use fenced code blocks (` ``` ` to begin/end) and explicitly declare the language, like ` ```python ` or ` ```plain `
- Add blank lines around headings, lists, and code blocks
- No trailing whitespace on lines
  - Use the `\` character at the end of a line for a single-line break, not the two-space syntax which is easy to miss
- Line limit: wrap lines at 80 characters; exceptions for links, tables, headings, and code blocks

### Docusaurus

Specific guidelines for sites built with Docusaurus:

- Define the page title in front matter and repeat it as a matching Markdown H1. Sections within a page begin with Heading 2 (`##`). Follow [`docs/_docs-conventions.md`](../../../../docs/_docs-conventions.md).
- Use relative links that include the numbered filename prefix for cross-page links, such as `[CLI](./07-cli.md)`.
- Use the `.md` extension for pages under `docs/`.
- Use the front matter section on all pages. At a minimum, set the `title` (this is rendered into the page as an H1) and a short `description`. [[Reference](https://docusaurus.io/docs/api/plugins/@docusaurus/plugin-content-docs#markdown-front-matter)]
- Use titles and line highlights in code blocks to provide context and improve readability. [[Reference](https://docusaurus.io/docs/markdown-features/code-blocks)]
  - Titles are added using the `title="..."` attribute in the opening code fence.
  - Line highlights are added using comma-separated `{number}` or `{start-end}` ranges in the opening code fence, or `highlight-next-line`, `highlight-start`, and `highlight-end` comments within the code block.
- Use admonitions for notes, tips, warnings, and other annotations. This provides a consistent look and feel across the site. [[Reference](https://docusaurus.io/docs/markdown-features/admonitions)]
  - Use square brackets to add a custom title, e.g. `:::info[My title]`.
  - Add empty lines around the start and end directives to avoid formatting issues with Prettier.
  - Don't overuse admonitions; they are best for callouts that add value beyond the main content. Too many admonitions can become visually overwhelming and interrupt the flow of documentation.
  - An admonition must add information beyond the surrounding text. Don't use one to restate or negate what the adjacent prose already says.
- Place images in `static/img` using WebP, PNG, or SVG format.
- Use the [`ThemedImage` component](https://docusaurus.io/docs/markdown-features/assets#themed-images) to provide both light and dark mode screenshots for apps/UIs that support both.

## Products and projects

These are the products and projects Mecatl documentation may need to mention.

**ToolHive**: An open source runtime for deploying and operating MCP servers. Write it as one bi-capitalized word (not "Toolhive" or "Tool Hive").

**Mecatl**: An open source, cloud-native agent harness for running AI agents as production workloads on infrastructure you operate. Style Mecatl as a proper noun. Write the component and binary names `mecatui`, `mecated`, `mecak8s`, and `mecatequi` in lowercase code font.

## Word list & glossary

Common terms used in Mecatl documentation:

**open source**: We prefer using two words over the hyphenated form (not "open-source"). It's not a proper noun, so don't capitalize unless it starts a sentence.

**cloud-native harness**: A generic architectural description, not a proper noun. Keep it lowercase and hyphenate "cloud-native" when it modifies "harness."

**OSS**: Abbreviation for "open source software".

**Stacklok**: The company behind Mecatl and ToolHive. It's written as one word with a single capital (not "StackLok" or "Stacklock").

### Products/brands

**Copilot** - GitHub's AI coding assistant. It's written with only a leading capital (not "CoPilot").

**Git**: The most popular distributed version control system. It underpins most commercial VCS offerings like GitHub, Bitbucket, and GitLab. Unless specifically referring to the `git` command-line tool, it's a proper noun and should be capitalized.

**GitHub**: The most popular source code hosting provider, especially for open source. It's written bi-capitalized as one word (not "Git Hub" or "Github").

**JetBrains**: A company that makes IDEs for many languages, including IntelliJ IDEA, PyCharm, GoLand, and more. It's written bi-capitalized as one word (not "Jet Brains" or "Jetbrains"). It's proper to reference a specific JetBrains IDE when needed, or simply refer to "all JetBrains IDEs".

**LLM**: large language model, a type of machine learning model designed for natural language processing tasks. LLM is an abbreviation, so it's written in all caps. Written out, it is lower-cased.

**MCP**: Model Context Protocol. MCP is an open protocol that standardizes how applications provide context to LLMs. MCP is an abbreviation, so it's written in all caps. Written out, it is proper-cased.

**Microsoft Entra ID**: Microsoft's cloud identity and access management platform. Microsoft rebranded it from "Azure AD" (Azure Active Directory) in 2023, so don't use "Azure AD" except when quoting a literal API, CLI, or field value that still uses the old name. Use "Microsoft Entra ID" on first reference, "Entra ID" thereafter.

**npm**: The registry for JavaScript packages (the "npm registry"), and the default package manager for JavaScript. Since it's both the registry _and_ the package manager, it may be useful to disambiguate "the npm registry". It's not an abbreviation, so it's not capitalized; it's written all lowercase (not "NPM").

**OpenAI**: The company behind the GPT models and ChatGPT. It's written bi-capitalized as one word (not "Open AI" or "Openai").

**Visual Studio Code**: A popular free integrated development environment (IDE) from Microsoft. Per Microsoft's [brand guidelines](https://code.visualstudio.com/brand#brand-name), use the full "Visual Studio Code" name the first time you reference it. "VS Code" is an acceptable short form after the first reference. It's written as two words and there are no other abbreviations/acronyms (not "VSCode", "VSC", or just "Code").
