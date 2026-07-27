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

**`<` and `>` outside a literal region become `&lt;` and `&gt;`.** That is the
whole security rule. A character reference is text to every Markdown parser; it
cannot become a tag.

**Inside a literal region nothing is escaped.** Markdown already treats a code
span's contents and a code block's lines literally, so escaping there would put
a visible `&lt;x&gt;` on the page — the fidelity bug this decision exists to
avoid, in the other direction.

**`github.com/yuin/goldmark` decides which regions those are.** The first
attempt tracked code spans with a character scanner, and a scanner is not a
parser: it read a backslash-escaped `` \` `` as a delimiter and let a span run
across a paragraph break, declaring raw HTML literal in two ways CommonMark
never would, and it escaped inside `~~~` fences and indented blocks. Each corner
was found by someone looking for it. The next one would have shipped live.

**Only the parse is borrowed, never the rendering.** goldmark locates the
literal regions and the escaping writes back into the original bytes at those
offsets. Running goldmark's renderer would normalize and reflow prose the author
wrote and rewrite every committed `.md`; the parser is also assembled directly
from `parser.NewParser`, so the HTML renderer never enters the binary.

**Where a value lands decides what is literal there.** A description emitted as
its own block can hold a code block. A table cell, a heading or a list item
cannot open one partway through a line, so a value that looks like a fence when
parsed alone is ordinary text in the document — treating it as code would leave
the HTML behind it live.

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

## Considered and rejected

- **Harden the scanner.** Teach it backslash escapes, then blank lines, then
  fences. That is writing a CommonMark parser one bug report at a time, and the
  failure mode of getting it wrong is a live tag on a published page.
- **Escape every `<` and `>`, code spans included.** Simple and safe, and it
  puts a visible `&lt;` in every model that backticks a generic type. The
  fidelity loss is the thing this decision is about.
- **Render through goldmark and emit its output.** It would be correct by
  construction and it would rewrite the author's prose — reflowed paragraphs,
  normalized emphasis, churn in every committed `.md` on every upgrade of the
  library.

## Consequences

- **modelith takes its first parsing dependency.** It ships as a single static
  binary, so this is not free: goldmark is pure Go (no `import "C"`, builds
  under `CGO_ENABLED=0`), only its `ast`, `parser`, `text` and `util` packages
  link in, and the binary grows from 6.26 MB to 6.89 MB. Its only `net` import
  is `net/url`, which `TestADR_0011_OfflinePackages` allows as a parser that
  performs no I/O; the offline boundary is unchanged.
- **A code block in a block-level field reaches the page verbatim.** A `~~~`
  fence or an indented block in a `description:` or `definition:` is code, and
  is left alone. In a table cell or a list item the same text is not, because it
  could not open a block there.
- **An entity `derivation:` is collapsed onto one line.** It renders after
  `**Derived:** `, so it is a line, not a block, and is escaped as one.
- **An author cannot embed raw HTML in a model.** No model did, and a domain
  model is not a place to hand-write markup. Someone who needs a construct
  Markdown lacks has to ask for it, which is the right conversation to have.
- **An autolink written as `<https://example.com>` renders as text.** Bare URLs
  still autolink under GFM, so the loss is the pointy-bracket form only —
  and with it goes `<javascript:alert(1)>`, which CommonMark would otherwise
  turn into a live link.
- **A Markdown link is still a Markdown link.** `[click](javascript:alert(1))`
  in prose passes through, because it is Markdown and this decision is about
  raw HTML. Whether a destination's scheme should be constrained is a separate
  question and is not decided here.
- **MDX expression syntax is untouched.** A `{` in prose still reaches the
  Docusaurus build as an expression. That is a separate class from raw HTML and
  is not decided here.
- **No committed golden changed.** Nothing in `examples/` or
  `docs/05-parking-garage/` holds an angle bracket or an ampersand in a prose
  field, so this landed as a pure behavior change with no output churn — which
  also means the goldens do not demonstrate it.
  `TestADR_0014_ProseRendersHTMLAsText` walks every prose-bearing field through
  a render; `TestADR_0014_NoRawHTMLSurvivesRender` asks a second CommonMark
  implementation whether anything survived; `TestADR_0014_BlockCodeStaysVerbatim`
  pins the other direction.
