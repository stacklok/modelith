# Writing tutorials

[Technical writing skill](../SKILL.md)

A tutorial is a lesson. The reader is a learner who wants to acquire skill by doing something under your guidance. They don't yet know what they need to know, so you, the teacher, carry all responsibility for their success. If a learner follows the steps exactly and something fails, the tutorial failed, not the learner.

Tutorials are typically the quickstarts inside each product section and any end-to-end getting-started pages. A quickstart should deliver a working result in under 10 minutes.

## Principles

**Deliver visible results early and often.** Every step, or small group of steps, should produce something the learner can see: output, a running container, a response from a tool. Visible progress builds the confidence that keeps a learner going. Never string together long sequences of setup with nothing to show for it.

**Get the learner started, not educated.** The goal is a completed experience and earned confidence, not comprehensive knowledge. Resist covering options, alternatives, and edge cases. There is one path through a tutorial, and you choose it.

**Minimize explanation.** A learner mid-task cannot absorb theory; explanation interrupts the doing. Offer only the minimum context a step needs ("You need Docker because ToolHive runs MCP servers in containers"), and link to concept pages for anything deeper. If you find yourself writing paragraphs of background, that content belongs in an explanation page.

**Give no choices.** "You can use X or Y" is poison in a tutorial. The learner has no basis for choosing and every fork doubles the ways the lesson can go wrong. Pick one client, one server, one installation method. Alternatives belong in how-to guides.

**Be concrete and specific.** Real commands, real components, real output. Show the learner what they should see after each significant step ("You should see the server listed with status `running`") so they can confirm they're on track before continuing.

**Guarantee repeatability.** The tutorial must work, exactly as written, for every learner in a reasonable environment, every time. This is the hardest and most important obligation. State prerequisites completely, pin anything that drifts, and test the steps end to end before publishing.

**Signpost the journey.** Tell the learner what they'll accomplish at the start ("In this tutorial, you'll run your first MCP server and connect it to VS Code"), mark progress along the way, and close by naming what they achieved.

## Keep out of tutorials

- Explanations longer than a sentence or two (link to concept pages instead)
- Options, alternatives, and configuration surveys (how-to material)
- Complete listings of flags or fields (reference material)
- Troubleshooting content beyond the one or two failures learners actually hit, placed in a closing Troubleshooting section, never inline
- Abstractions and generalization; the lesson is this concrete task

## Structure

1. Front matter: `title` and `description` (see the style guide)
2. What you'll do and what you'll have at the end, in a sentence or two
3. Prerequisites, complete and verifiable
4. Numbered steps, each with a visible result
5. A closing recap of what the learner accomplished
6. Next steps (1-3 links: the natural follow-on guides)
7. Related information, then Troubleshooting, if applicable

## Voice notes

Diataxis suggests first person plural ("we") for tutorials; Stacklok docs use second person ("you") everywhere instead, per the style guide. Keep the teacher's encouraging, confident tone without the "we": "You'll notice the server appears in the list", "Now connect your client."
