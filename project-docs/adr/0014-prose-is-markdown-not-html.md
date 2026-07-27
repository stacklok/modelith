# A prose field is Markdown, not HTML

A `definition:`, `description:`, `role:`, `note:`, invariant `statement:` or
scenario step is rendered as Markdown, and its raw HTML is rendered as visible
text rather than interpreted. An angle bracket the author typed appears on the
page as an angle bracket. Markdown — emphasis, and the backticked entity names
these fields are written in throughout — keeps working.

## Context

The renderer wrote prose fields into the Markdown document verbatim. Markdown
allows inline HTML, so `<img src=q onerror=alert(1)>` in a `definition:` became a
tag in the published page, and no lint severity flagged it.

Passing Markdown through is deliberate and has to stay. `lint`'s backticked-term
check expects prose to name entities in code spans, the schema tells authors to
backtick entity names in a role and an invariant statement, and the worked
example relies on both. So the choice was never "escape prose or don't" — it was
where inside prose the line falls.

Today the hole is cosmetic: you wrote the model you render. ADR-0010's vendoring
slice renders models copied from repositories this one does not control into
local docs and a published site. At that point the author of the string needs
commit access to *their* repo, not yours, and this becomes a trust boundary.
ADR-0011 keeps `render` offline, so nothing else stands between a vendored file
and the page.

## Decision

**`<` and `>` outside a code span become `&lt;` and `&gt;`.** That is the whole
security rule. A character reference is text to every Markdown parser; it cannot
become a tag.

**Inside a code span nothing is escaped.** Markdown already treats a code span's
contents literally, so escaping there would put a visible `&lt;x&gt;` on the
page — the fidelity bug this decision exists to avoid, in the other direction.
The pass tracks code spans by CommonMark's rule: a run of *n* backticks closes at
the next run of exactly *n*, and an unclosed run is ordinary text.

**An `&` is escaped only where it introduces a character reference** —
`&name;`, `&#60;`, `&#x3c;`. This is fidelity, not safety: a reference can never
produce markup, because a Markdown parser decodes one to a character and
re-escapes it on output. Escaping it makes an author who wrote `&lt;` see
`&lt;`, the same rule as one who wrote `<`. Leaving a bare `&` alone means
`R&D`, `a & b` and a query string render as themselves and pass into the
committed Markdown byte-for-byte, which keeps the generated file readable.

The Mermaid renderer reaches the same contract by a different encoding. Mermaid
builds labels as HTML and decodes references back to characters, so `sanitize`
escapes *every* `&` along with `<` and `>` — a round trip that never surfaces,
rather than a visible escape. Both renderers answer the same question the same
way: the reader sees the characters the author typed.

## Consequences

- **An author cannot embed raw HTML in a model.** No model did, and a domain
  model is not a place to hand-write markup. Someone who needs a construct
  Markdown lacks has to ask for it, which is the right conversation to have.
- **An autolink written as `<https://example.com>` renders as text.** Bare URLs
  still autolink under GFM, so the loss is the pointy-bracket form only.
- **MDX expression syntax is untouched.** A `{` in prose still reaches the
  Docusaurus build as an expression. That is a separate class from raw HTML and
  is not decided here.
- **No committed golden changed.** Nothing in `examples/` or
  `docs/05-parking-garage/` holds an angle bracket or an ampersand in a prose
  field, so this landed as a pure behavior change with no output churn — which
  also means the goldens do not demonstrate it. `TestADR_0014_ProseRendersHTMLAsText`
  does, walking every prose-bearing field through a render.
