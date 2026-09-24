# Writing explanation

[Technical writing skill](../SKILL.md)

Explanation is a discussion. The reader is away from the keyboard, or at least away from the task, and wants to deepen their understanding: how does this work, why is it designed this way, how does it relate to the alternatives? Explanation is the mode that serves study rather than work, and it's the only mode where context, background, opinion, and trade-offs belong.

Explanation typically lives in a concepts section (protocol primers, architecture overviews, security frameworks) and in each product section's Introduction page, which explains what the product is and who it's for.

## Principles

**Answer "why", not "how".** Explanation provides the reasoning, history, constraints, and mental models behind the machinery: why ToolHive runs MCP servers in containers, how the authorization pieces fit together, when vMCP makes sense. The moment you're writing numbered steps, you've left the mode.

**Bound the topic.** A useful test: the page should make sense with "About" in front of its title ("About network isolation"). If it wouldn't, the scope is fuzzy. Say early what the page covers, and keep it there; explanation has no natural task boundary, so unbounded pages sprawl.

**Make connections.** Explanation earns its keep by joining things: how a feature relates to its neighbors, to the broader ecosystem, to what the reader already knows from elsewhere. Comparisons, context, and even relevant history (why the ecosystem settled on a pattern) are welcome here in a way they're welcome nowhere else. Weigh that against the "document current behavior" rule in the style guide: product-version changelog narration is still out, but design rationale and ecosystem background are fair game.

**Discuss trade-offs and admit alternatives.** Explanation is where "when to use X vs. Y" content belongs, with honest weighing rather than a sales pitch. State limitations accurately and neutrally; don't dramatize them, and don't paper over them.

**Opinion is allowed; keep it justified.** Perspectives and recommendations are legitimate here ("for production deployments, the operator is the better fit because..."), provided the reasoning comes with them.

**Serve understanding, not completeness.** Explanation doesn't need to mention every field or cover every case; that's reference's job. Choose the details that build the mental model and link out for the rest.

## Keep out of explanation

- Step-by-step instructions (link to the how-to guide)
- Exhaustive technical description, field lists, option tables (link to reference)
- Anything the reader must do; explanation should be safely skippable by someone who just wants to get the task done

## Structure

Explanation is prose-first. Headings mark the major facets of the topic, and paragraphs, not bullet fragments, carry the reasoning; connected argument is the whole point of the mode, and bullets break the connections. Diagrams (Mermaid) help when the relationships are structural. Close with Next steps or Related information pointing to the quickstart or guides where the reader applies the understanding.
