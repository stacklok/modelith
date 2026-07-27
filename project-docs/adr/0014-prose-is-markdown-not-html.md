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

**Escape the bytes a parser calls markup, and leave every other byte alone.**
The rule is an allowlist of what to escape rather than of what to spare: the
byte ranges of the raw-HTML nodes — `ast.RawHTML`, `ast.HTMLBlock` — have their
`<`, `>` and `&` replaced by character references, and nothing else in the
string is touched. A character reference is text to every Markdown parser; it
cannot become a tag.

**The inverse rule was tried first, and it corrupted the document.** Collecting
the literal regions and escaping everything outside them meant escaping Markdown
*structure*. A `>` is a blockquote marker, so `&gt;` stopped opening one: the
indented code block inside the quote — which the same parse had just called
literal — collapsed into a paragraph and shipped its contents live. Escaping
changed how the text parsed, which invalidated the offsets it was writing at. It
also killed the `<...>` around a link destination, leaving a dead href. Escaping
only what a parser recognised as markup has no such feedback: it removes angle
brackets and never adds one, so it cannot disturb the parse it came from.

**`github.com/yuin/goldmark` decides what is markup.** The first attempt tracked
code spans with a character scanner, and a scanner is not a parser: it read a
backslash-escaped `` \` `` as a delimiter and let a span run across a paragraph
break, calling raw HTML literal in two ways CommonMark never would. Each corner
was found by someone looking for it. The next one would have shipped live.

**Only the parse is borrowed, never the rendering.** goldmark locates the markup
and the escaping writes back into the original bytes at those offsets. Running
goldmark's renderer would normalize and reflow prose the author wrote and
rewrite every committed `.md`; the parser is also assembled directly from
`parser.NewParser`, so the HTML renderer never enters the binary.

**A code fence's info string counts as markup.** It reaches the page as a class
attribute rather than as text, and it is inert to block structure, so escaping
it cannot disturb the parse and it keeps an unclosed fence from carrying a tag
on its opening line.

**A rendered line is escaped once, as the whole line.** Whether a backtick opens
a code span is a property of the finished line, not of the field it came from. A
`role:` and the `note:` beside it are separate schema fields that share one
bullet, and escaping them separately let a stray backtick in the role pair with
the note's backticks and leave the note's tag outside every span, live. So the
line is assembled first, generated markup included, and escaped as the reader's
parser will see it. Table cells stay separate, because GFM splits a row into
cells before it parses any of them.

**Where a value lands decides how it is parsed.** A description emitted as its
own block can hold a code block. A value on a line always follows something —
`- `, `# `, `| ` — so a leading four-space indent or a ` ``` ` run cannot open a
code block there. A line-context value is therefore parsed behind a stand-in for
that text: parsed alone it would look like a code block, and a parser reports
nothing inside one as HTML.

**An `&` outside a tag is left alone.** Escaping the ones that introduced a
character reference was fidelity, not safety — a reference decodes to a
character and is re-escaped on output, so it can never produce markup — and it
is not worth a rule that reaches beyond the markup. `R&D`, `a & b` and a query
string pass into the committed Markdown byte-for-byte. Inside a tag the `&` is
escaped along with the brackets, so the tag reaches the page as it was typed.

The Mermaid renderer reaches the same contract by a different encoding. Mermaid
builds labels as HTML and decodes references back to characters, so `sanitize`
escapes *every* `&` along with `<` and `>` — a round trip that never surfaces,
rather than a visible escape. Both renderers answer the same question the same
way: the reader sees the characters the author typed.

## Considered and rejected

- **Harden the scanner.** Teach it backslash escapes, then blank lines, then
  fences. That is writing a CommonMark parser one bug report at a time, and the
  failure mode of getting it wrong is a live tag on a published page.
- **Escape everything outside the literal regions.** The parse says what is
  literal, so the complement looked like a safe default. It is not: the
  complement contains the document's structure, and escaping structure changes
  the parse the offsets came from. See the Decision above.
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
  link in, and the binary grows from 6.26 MB to 6.89 MB. Its `net` imports are
  `net/url` and, through it, `net/netip` — parsers that perform no I/O, which
  `TestADR_0011_OfflinePackages` allows by name. The offline boundary is
  unchanged.
- **Markdown structure survives, because none of it is escaped.** A blockquote
  stays a blockquote, `[click](<http://example.com/a>)` keeps its href, and
  `<https://example.com>` stays an autolink.
- **A code block in a block-level field reaches the page verbatim.** A `~~~`
  fence or an indented block in a `description:` or `definition:` is code, and
  is left alone. In a table cell or a list item the same text is not, because it
  could not open a block there.
- **An angle bracket that opens no tag is not markup and is not escaped.**
  `map<string, int>` and `1 < 2 && 3 > 2` reach the committed `.md` as typed and
  render as themselves.
- **An author who wrote `&lt;` now sees `<`.** The reference is decoded on the
  page rather than shown. This is the fidelity the `&` rule used to buy, given
  up to keep the escaping inside the markup.
- **An entity `derivation:` is collapsed onto one line.** It renders after
  `**Derived:** `, so it is a line, not a block, and is escaped as one.
- **An author cannot embed raw HTML in a model.** No model did, and a domain
  model is not a place to hand-write markup. Someone who needs a construct
  Markdown lacks has to ask for it, which is the right conversation to have.
- **goldmark is handed a trailing newline it did not get from the author.** It
  compares an indent measured in columns against a line's length in bytes, so a
  final line a tab can outrun is taken for a blank one and the fenced-code-block
  parser indexes it at the -1 that marks one — a panic on a model that lints
  clean. Upstream yuin/goldmark#556 is the same -1 by another path; v1.8.4
  carries that fix and still panics on #556's own repro, so there is no release
  to upgrade to. Appending the line ending a document is defined to have moves
  no offset inside the string. `TestRender_UnnormalisedProseDoesNotPanic` pins
  it.
- **No committed golden changed.** Nothing in `examples/` or
  `docs/05-parking-garage/` holds an angle bracket or an ampersand in a prose
  field, so this landed as a pure behavior change with no output churn — which
  also means the goldens do not demonstrate it.
  `TestADR_0014_ProseRendersHTMLAsText` walks every prose-bearing field through
  a render; `TestADR_0014_NoRawHTMLSurvivesRender` asks a second parser whether
  anything survived; `TestADR_0014_BlockCodeStaysVerbatim` pins the other
  direction; `TestADR_0014_AssembledLineEscapesAsOneLine` pins the shared line.

## Not decided here

Each of these is a different class from raw HTML. Naming them is the point: the
rule above is about what a parser calls a tag, and none of these is one.

- **A link's scheme.** `[click](javascript:alert(1))` and the autolink
  `<javascript:alert(1)>` both pass through, because a destination is Markdown
  and not raw HTML. The allowlist made the autolink form survive where the
  earlier rule had incidentally broken it, so this is now the live question
  rather than a latent one. Constraining schemes is its own decision, with its
  own list of what to allow, and it is not taken here.
- **MDX.** The docs are built by Docusaurus, whose MDX parser reads `<` followed
  by a name character as a JSX tag. `map<string, int>` and `<https://example.com>`
  both fail an `@mdx-js/mdx@3` compile, while `1 < 2 && 3 > 2` and an escaped
  `&lt;img src=q&gt;` both pass. A `{` in prose still reaches the build as an
  expression. This is unchanged from before the escaping existed — the same
  bytes reach the page as they did — but the escaping does not close it either.
- **A prose block that leaves a code fence open.** A `definition:` of
  ` ``` ` and then `<A onerror=alert(1)>` parses, on its own, as an unclosed
  fenced block whose contents are code — so nothing in it is markup. In the
  assembled document that fence pairs with the next one, which is the Mermaid
  diagram's, and the tag lands outside any block and goes live. The field's own
  parse is only authoritative while the field is self-contained, and an open
  fence is exactly the case where it is not. The bytes are unchanged from before
  the escaping existed. Refusing an unbalanced fence reads as a lint rule about
  a malformed prose block rather than as a renderer escaping decision, and which
  it should be is not settled here.
- **The parser configuration is CommonMark; every reader's is GFM.** GitHub and
  Docusaurus both apply the GFM extensions, and GFM's table transformer can
  dissolve a paragraph into a table, so a CommonMark code span that spanned a
  newline never forms and its contents land live. `goldmark/extension` cannot be
  added for its parsers alone: every file in it imports the root `goldmark`
  package for the `Extend` signature, whose eager `defaultMarkdown` global drags
  `renderer` and `renderer/html` into the binary — measured, 27 renderer symbols
  against zero today. Closing this means vendoring the table transformer into a
  renderer-free package, which is a dependency-surface decision of its own.
