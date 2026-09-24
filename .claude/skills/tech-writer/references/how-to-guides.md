# Writing how-to guides

[Technical writing skill](../SKILL.md)

A how-to guide is a recipe. The reader is a competent user with a real task in front of them: they know what they want to achieve and roughly what they're doing, and they need reliable directions for this specific goal. Unlike a tutorial's learner, they can adapt, fill small gaps, and recover from minor surprises. Respect that competence.

How-to guides are usually the bulk of a documentation set: the task-oriented guide pages for each product area and the third-party integration guides.

## Principles

**Address the reader's problem, not the tool's features.** Name the page after the task ("Customize permission profiles", "Send telemetry to Datadog"), not the mechanism ("The --permission-profile flag"). Organize by what the reader is trying to do; a guide that walks through a feature's options in the order the code defines them is reference material wearing the wrong hat.

**Solve one task per page.** A guide that covers several loosely related tasks serves none of them well and is hard to find. If a page needs "and" in its title, it probably needs splitting.

**Prefer usability over completeness.** A how-to guide gets the reader to a working result; it doesn't enumerate everything the feature can do. Cover the common path thoroughly, cover the most likely failure the reader will hit, and link to reference material for the full option surface. An incomplete guide the reader can follow beats a complete one they can't.

**Sequence the actions.** The heart of a guide is an ordered series of steps toward the goal. Keep each step an action; put decisions the reader must make at the point they must make them, with just enough context to choose ("Use the `sse` transport if your client doesn't support streamable HTTP").

**Omit teaching and background.** The reader is working, not studying. A sentence of orientation is fine; paragraphs of theory are not. Link to concept pages for the why and to reference material for the details, and keep the guide moving.

**Make prerequisites operational.** Don't just list what must exist; say what state it must be in and what that implies ("keep this command running in a separate terminal while you complete the next section").

**Scope to this project's job.** For guides involving third-party tools or services, the guide's job is getting the thing working with this product. Upstream quirks and caveats get at most a line and a link to the upstream docs; reproducing upstream documentation makes the page long and instantly stale.

## Keep out of how-to guides

- Conceptual explanation beyond a sentence of orientation (link to concept pages)
- Exhaustive flag, field, or option listings (link to reference material)
- Teaching-style hand-holding ("congratulations!", restating what a command obviously does)
- Multiple unrelated tasks
- Upstream product documentation restated at length

## Structure

1. Front matter: `title` and `description` (see the style guide)
2. A sentence or two on what the guide accomplishes and when you'd want it
3. Prerequisites, with operational context
4. The steps, organized by the reader's workflow; use tabs (where the site supports them) for genuinely parallel variants (UI vs. CLI, macOS vs. Windows), not for optional extras
5. Verification: how the reader confirms it worked
6. Next steps (required: 1-3 links following the reader's journey, for example: install, use, secure, operate, optimize)
7. Related information, then Troubleshooting, in that order, if applicable
