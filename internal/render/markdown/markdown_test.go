package markdown

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/stacklok/modelith/internal/model"
)

// gfm parses rendered output the way a reader's tooling will: CommonMark plus
// the GFM extensions, tables above all, since most of what this renderer emits
// is a table.
//
// Every assertion about escaping in this file goes through it rather than
// through a hand-written scan of the output. The renderer used to carry its own
// backtick matcher, and the test that guarded it reimplemented the same
// matching — so the guard shared the implementation's model of Markdown and
// agreed with it about an escaped "\`" and about a span running across a
// paragraph break, both wrong, both shipping live HTML (#37). A test that
// reuses the implementation's assumptions proves nothing; this one asks a
// second implementation.
var gfm = goldmark.New(goldmark.WithExtensions(extension.GFM))

func parseRendered(md string) (ast.Node, []byte) {
	src := []byte(md)
	return gfm.Parser().Parse(text.NewReader(src)), src
}

// rawHTML returns every raw-HTML fragment a real parser finds in md. ADR-0014's
// rule is that a rendered model holds none: an angle bracket the author typed
// reaches the page as text.
func rawHTML(t *testing.T, md string) []string {
	t.Helper()
	doc, src := parseRendered(md)
	var out []string
	segments := func(segs *text.Segments) {
		for i := range segs.Len() {
			s := segs.At(i)
			out = append(out, string(src[s.Start:s.Stop]))
		}
	}
	walk(t, doc, func(n ast.Node) ast.WalkStatus {
		switch node := n.(type) {
		case *ast.RawHTML:
			segments(node.Segments)
		case *ast.HTMLBlock:
			segments(node.Lines())
			if node.HasClosure() {
				out = append(out, string(src[node.ClosureLine.Start:node.ClosureLine.Stop]))
			}
		}
		return ast.WalkContinue
	})
	return out
}

// textOutsideCode returns the document's text with every code span and code
// block skipped, walking the parse tree rather than rescanning backticks. A
// payload that appears here escaped the span that was supposed to hold it.
//
// A link destination is not part of it. A destination is percent-encoded by
// linkTarget, not escaped for prose, so an encoded payload would read as a
// false positive here; the destinations are asserted exactly, by hand, at the
// call sites that produce them.
func textOutsideCode(t *testing.T, md string) string {
	t.Helper()
	doc, src := parseRendered(md)
	var b strings.Builder
	walk(t, doc, func(n ast.Node) ast.WalkStatus {
		switch node := n.(type) {
		case *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren
		case *ast.Text:
			b.Write(node.Segment.Value(src))
			b.WriteString("\n")
		case *ast.AutoLink:
			b.Write(node.URL(src))
			b.WriteString("\n")
		}
		return ast.WalkContinue
	})
	return b.String()
}

// links counts the link nodes in md. A destination that closed early and opened
// a second link changes the count, whatever the label says.
func links(t *testing.T, md string) int {
	t.Helper()
	doc, _ := parseRendered(md)
	n := 0
	walk(t, doc, func(node ast.Node) ast.WalkStatus {
		switch node.(type) {
		case *ast.Link, *ast.AutoLink, *ast.Image:
			n++
		}
		return ast.WalkContinue
	})
	return n
}

// headings returns the text of every heading in md, in document order. A value
// that broke out of its line and opened a section shows up as an extra entry.
func headings(t *testing.T, md string) []string {
	t.Helper()
	doc, src := parseRendered(md)
	var out []string
	walk(t, doc, func(n ast.Node) ast.WalkStatus {
		if h, ok := n.(*ast.Heading); ok {
			out = append(out, nodeText(h, src))
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})
	return out
}

// nodeText concatenates the source text of n's inline descendants, code spans
// included. ast.Node.Text is deprecated, and it would drop the span that every
// entity heading is written in.
func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
			continue
		}
		b.WriteString(nodeText(c, src))
	}
	return b.String()
}

// tableRows returns the cell count of every row of every table in md, header
// row first. A pipe that escaped its cell changes a count; counting pipes in
// the raw line cannot tell an escaped one from a structural one.
func tableRows(t *testing.T, md string) []int {
	t.Helper()
	doc, _ := parseRendered(md)
	var out []int
	walk(t, doc, func(n ast.Node) ast.WalkStatus {
		switch n.(type) {
		case *extast.TableHeader, *extast.TableRow:
			cells := 0
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if _, ok := c.(*extast.TableCell); ok {
					cells++
				}
			}
			out = append(out, cells)
		}
		return ast.WalkContinue
	})
	return out
}

func walk(t *testing.T, doc ast.Node, visit func(ast.Node) ast.WalkStatus) {
	t.Helper()
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		return visit(n), nil
	})
	if err != nil {
		t.Fatalf("walking the parsed document: %v", err)
	}
}

// render calls Render with sourceDir == outDir, the default beside-the-source
// case every test in this file exercises except the ones specifically about
// relativizing an import link against a different output directory (see
// TestRenderImports_LinkRelativeToOutputDir). The two directories are never
// read from disk — Render only uses them to compute relative import links —
// so any distinct absolute-looking path pair does the job.
func render(m *model.Model) string {
	return Render(m, "/src", "/src")
}

// firstDiff reports the first line where want and got differ, so a golden
// failure points at the change instead of just saying "they differ."
func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) > n {
		n = len(gl)
	}
	for i := 0; i < n; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n  want: %q\n  got:  %q", i+1, w, g)
		}
	}
	return "(no line-level difference found)"
}

// TestRenderInvariantsSection checks the model-level invariants render as a
// top-level "## Invariants" section, and that the section is omitted entirely
// when there are none (per-entity invariants render with their entity instead).
func TestRenderInvariantsSection(t *testing.T) {
	with := &model.Model{
		Entities: map[string]model.Entity{
			"Project": {Definition: "A container."},
		},
		Invariants: []model.Invariant{
			{ID: "cross-entity-rule", Statement: "Spans the `Project` and the `Policy`."},
		},
	}
	got := render(with)
	if !strings.Contains(got, "## Invariants\n") {
		t.Fatalf("expected a top-level Invariants section, got:\n%s", got)
	}
	if !strings.Contains(got, "- **cross-entity-rule** — Spans the `Project` and the `Policy`.") {
		t.Fatalf("expected the invariant bullet, got:\n%s", got)
	}

	without := &model.Model{
		Entities: map[string]model.Entity{
			"Project": {Definition: "A container."},
		},
	}
	if strings.Contains(render(without), "## Invariants") {
		t.Fatalf("did not expect an Invariants section when there are none:\n%s", render(without))
	}
}

// TestRenderEntity_DerivedMarker checks that a derived entity shows a clear
// "Derived" marker with its derivation when present, and a generic marker
// when derivation is omitted (mirroring the derived-attribute rendering).
func TestRenderEntity_DerivedMarker(t *testing.T) {
	withDerivation := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {
				Definition: "A ranked view over current scores.",
				Derived:    true,
				Derivation: "Computed on demand from `Score` records.",
			},
		},
	}
	got := render(withDerivation)
	if !strings.Contains(got, "**Derived:** Computed on demand from `Score` records.\n\n") {
		t.Fatalf("expected a derivation-specific marker, got:\n%s", got)
	}

	withoutDerivation := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {
				Definition: "A ranked view over current scores.",
				Derived:    true,
			},
		},
	}
	got = render(withoutDerivation)
	if !strings.Contains(got, "**Derived.** Computed on demand from other state; never persisted.\n\n") {
		t.Fatalf("expected a generic derived marker when derivation is omitted, got:\n%s", got)
	}

	notDerived := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {Definition: "A ranked view over current scores."},
		},
	}
	if strings.Contains(render(notDerived), "**Derived") {
		t.Fatalf("did not expect a derived marker on a non-derived entity:\n%s", render(notDerived))
	}
}

// TestRenderEntity_SymmetricRelationship pins where a `symmetric` marker
// surfaces. The ER diagram has no notation for it (ADR-0008 leaves the line to
// ownership and the label to the role), so the Markdown relationship line is
// the only place a reader sees it.
func TestRenderEntity_SymmetricRelationship(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"Node": {Definition: "A node.", Relationships: []model.Relationship{
			{Entity: "Node", Cardinality: "n:n", Symmetric: true, Role: "`Peer`"},
			{Entity: "Node", Cardinality: "1:0..1", Role: "`Predecessor`"},
		}},
	}}
	got := render(m)
	if want := "- `Node` — n:n — symmetric — `Peer`\n"; !strings.Contains(got, want) {
		t.Fatalf("expected a symmetric marker %q, got:\n%s", want, got)
	}
	if want := "- `Node` — 1:0..1 — `Predecessor`\n"; !strings.Contains(got, want) {
		t.Fatalf("expected no marker on a non-symmetric relationship %q, got:\n%s", want, got)
	}
}

// TestGoldenExample renders the committed example and compares it to the
// checked-in Markdown. This is the same invariant `modelith render --check` enforces
// in CI: if you change the renderer or the example YAML, regenerate the .md.
func TestGoldenExample(t *testing.T) {
	const (
		src    = "../../../examples/example.modelith.yaml"
		golden = "../../../examples/example.modelith.md"
	)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := render(m)

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("rendered output does not match %s.\nRegenerate with: go run ./cmd/modelith render %s\n%s",
			golden, src, firstDiff(string(want), got))
	}
}

// TestRenderImports_SectionAndLinkedTypes checks the imports section and the
// deep link on a qualified attribute type. The renderer never opens an imported
// file: the scope comes from the import that bound it and the anchor from the
// heading format enums render with, so the output stays a pure function of this
// model (ADR-0010, ADR-0012).
func TestRenderImports_SectionAndLinkedTypes(t *testing.T) {
	t.Parallel()

	m := &model.Model{
		Imports: []model.Import{
			{Scope: "payments", Path: "../payments/payments.modelith.yaml", ScopeFromPath: true},
			{Scope: "billing", Path: "./legacy/pay-v2.modelith.yaml"},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "A temporary entry credential.",
				Attributes: []model.Attribute{
					{Name: "paidWith", Type: "payments.PaymentMethod"},
					{Name: "plan", Type: "billing.Plan"},
					{Name: "unbound", Type: "shipping.Carrier"},
					{Name: "issuedAt", Type: "timestamp"},
				},
			},
		},
	}
	got := render(m)

	for _, want := range []string{
		"## Imports\n",
		// The source path is the model file; the link leads to the rendered
		// Markdown beside it. Labelling the link with the .yaml path pointed the
		// reader at a file they would not land on.
		"- **`payments`** — `../payments/payments.modelith.yaml` ([rendered](../payments/payments.modelith.md))\n",
		"- **`billing`** — `./legacy/pay-v2.modelith.yaml` ([rendered](./legacy/pay-v2.modelith.md))\n",
		"| `paidWith` | [payments.PaymentMethod](../payments/payments.modelith.md#paymentmethod) |  |\n",
		"| `plan` | [billing.Plan](./legacy/pay-v2.modelith.md#plan) |  |\n",
		// A scope no import binds is a lint error; the renderer states it rather
		// than linking to a file it has no reason to believe exists.
		"| `unbound` | shipping.Carrier |  |\n",
		"| `issuedAt` | timestamp |  |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	if strings.Contains(render(&model.Model{Entities: m.Entities}), "## Imports") {
		t.Error("did not expect an Imports section when there are no imports")
	}
}

// TestRenderImports_LinkRelativeToOutputDir pins the R2-1 fix: `render -o
// <dir>/x.md` and `--stdout` used to emit import links relative to the
// *source* directory regardless of where the output landed, so a link into
// `outdir/payments.modelith.md` dangled — the file modelith actually wrote was
// `payments.modelith.md` beside the source. The link target must instead be
// the imported model's default rendered path (RenderedPath resolved against
// sourceDir) expressed relative to outDir. The second case has sourceDir and
// outDir share no ancestor but "/", the case an implementation that assumes a
// common prefix gets wrong.
func TestRenderImports_LinkRelativeToOutputDir(t *testing.T) {
	t.Parallel()

	m := &model.Model{
		Imports: []model.Import{
			{Scope: "payments", Path: "../payments/payments.modelith.yaml", ScopeFromPath: true},
			{Scope: "billing", Path: "./legacy/pay-v2.modelith.yaml"},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "A temporary entry credential.",
				Attributes: []model.Attribute{
					{Name: "paidWith", Type: "payments.PaymentMethod"},
				},
			},
		},
	}

	for _, tc := range []struct {
		name              string
		sourceDir, outDir string
		payments, billing string
	}{
		{
			// `-o ../out/x.md`: a sibling directory sharing an ancestor
			// ("/repo") with the source.
			name:      "sibling output directory",
			sourceDir: "/repo/models",
			outDir:    "/repo/out",
			payments:  "../payments/payments.modelith.md",
			billing:   "../models/legacy/pay-v2.modelith.md",
		},
		{
			name:      "no common ancestor beyond root",
			sourceDir: "/src/models",
			outDir:    "/var/output",
			payments:  "../../src/payments/payments.modelith.md",
			billing:   "../../src/models/legacy/pay-v2.modelith.md",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Render(m, tc.sourceDir, tc.outDir)
			for _, want := range []string{
				fmt.Sprintf("- **`payments`** — `../payments/payments.modelith.yaml` ([rendered](%s))\n", tc.payments),
				fmt.Sprintf("- **`billing`** — `./legacy/pay-v2.modelith.yaml` ([rendered](%s))\n", tc.billing),
				fmt.Sprintf("[payments.PaymentMethod](%s#paymentmethod)", tc.payments),
			} {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderImports_AbsolutePathIsNotRelativized pins that an absolute import
// path is rendered unchanged rather than joined against sourceDir. An
// absolute path is a lint error (imports must be relative), but render only
// runs structural validation, so this is reachable. Before the fix,
// importLinkTarget joined it against sourceDir whenever outDir differed from
// sourceDir, silently re-rooting it under sourceDir and producing a
// different, misleading link than the same model rendered with no -o.
func TestRenderImports_AbsolutePathIsNotRelativized(t *testing.T) {
	t.Parallel()

	m := &model.Model{
		Imports: []model.Import{
			{Scope: "payments", Path: "/etc/payments.modelith.yaml"},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "A parking ticket.",
				Attributes: []model.Attribute{
					{Name: "paidWith", Type: "payments.PaymentMethod"},
				},
			},
		},
	}

	for _, tc := range []struct {
		name              string
		sourceDir, outDir string
	}{
		{name: "default, no -o", sourceDir: "/repo/models", outDir: "/repo/models"},
		{name: "-o into a sibling directory", sourceDir: "/repo/models", outDir: "/repo/out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Render(m, tc.sourceDir, tc.outDir)
			for _, want := range []string{
				"- **`payments`** — `/etc/payments.modelith.yaml` ([rendered](/etc/payments.modelith.md))\n",
				"[payments.PaymentMethod](/etc/payments.modelith.md#paymentmethod)",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderImports_HostilePathCannotEscapeItsMarkup pins that an import path —
// author-supplied text that reaches a published page — is escaped for the exact
// position it lands in. A path is not validated by the schema beyond a minimum
// length, so the renderer has to be safe on its own; the linter's rejection of
// control characters is defence in depth, not the barrier.
func TestRenderImports_HostilePathCannotEscapeItsMarkup(t *testing.T) {
	t.Parallel()

	// Closes the link destination, then opens an image and a second link.
	const breakout = `./p.modelith.yaml) <img src=x onerror=alert(1)> [click me](#`

	m := &model.Model{
		Imports: []model.Import{
			{Scope: "payments", Path: breakout},
			{Scope: "piped", Path: "./a|b.modelith.yaml"},
			{Scope: "spaced", Path: "./a b.modelith.yaml"},
			// A newline would inject a heading into the Imports list; the derived
			// scope carries it too, since it comes from the same string.
			{Scope: "line\nbreak", Path: "./a\n## Injected\n\nb.modelith.yaml", ScopeFromPath: true},
			{Scope: "ticked", Path: "./a`b``c.modelith.yaml"},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "A stub.",
				Attributes: []model.Attribute{{Name: "paidWith", Type: "payments.PaymentMethod"}},
			},
		},
	}
	got := render(m)

	for _, want := range []string{
		// The path is a code span, so nothing in it opens a tag, and the link
		// beside it carries a fixed label with nothing to escape.
		"- **`payments`** — `./p.modelith.yaml) <img src=x onerror=alert(1)> [click me](#`" +
			` ([rendered](./p.modelith.yaml%29%20%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E%20%5Bclick%20me%5D%28%23.md))` + "\n",
		// A pipe survives in the destination as %7C rather than being escaped
		// for a table cell it is not in.
		"`./a|b.modelith.yaml` ([rendered](./a%7Cb.modelith.md))\n",
		"`./a b.modelith.yaml` ([rendered](./a%20b.modelith.md))\n",
		// Every control character is replaced before the value is written.
		"- **`line break`** — `./a ## Injected  b.modelith.yaml` ([rendered](./a%0A%23%23%20Injected%0A%0Ab.modelith.md))\n",
		// A path holding backticks widens the span's fence rather than closing
		// it early.
		"```./a`b``c.modelith.yaml``` ([rendered](./a%60b%60%60c.modelith.md))\n",
		// The deep link is assembled from escaped parts: escaping the finished
		// link instead would put a backslash inside the destination.
		"| `paidWith` | [payments.PaymentMethod](./p.modelith.yaml%29%20%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E%20%5Bclick%20me%5D%28%23.md#paymentmethod) |  |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	// Every breakout token that survives is inside a code span, where a real
	// parser reads it as text. Nothing of the payload may reach the document's
	// prose, and the injected heading may not become a section.
	if html := rawHTML(t, got); len(html) > 0 {
		t.Errorf("a path reached the page as live HTML %q:\n%s", html, got)
	}
	bare := textOutsideCode(t, got)
	for _, tok := range []string{"<img", "onerror", "click me", "Injected"} {
		if strings.Contains(bare, tok) {
			t.Errorf("%q escaped its code span into the document text:\n%s", tok, bare)
		}
	}
	if want := []string{"Domain Model", "Imports", "Entities", "Ticket", "Relationships"}; !slices.Equal(headings(t, got), want) {
		t.Errorf("headings = %q, want %q — a path opened a section:\n%s", headings(t, got), want, got)
	}
	// One "([rendered](…))" per import, plus the deep link on the qualified
	// attribute type. A destination that closed early would add another.
	if want := len(m.Imports) + 1; links(t, got) != want {
		t.Errorf("links = %d, want %d — a destination closed early:\n%s", links(t, got), want, got)
	}
	// The Imports section is one list: a value that broke out would end it.
	if n := strings.Count(got, "\n- **"); n != len(m.Imports) {
		t.Errorf("expected %d import bullets, got %d:\n%s", len(m.Imports), n, got)
	}
}

// TestCodeSpan_FencesAroundBackticks checks the CommonMark rule the imports
// section leans on: a span's fence is longer than any backtick run inside it, so
// a value holding backticks stays intact instead of closing the span early.
func TestCodeSpan_FencesAroundBackticks(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"payments", "`payments`"},
		{"a`b", "``a`b``"},
		{"a``b`c", "```a``b`c```"},
		{"`edge`", "`` `edge` ``"},
		{"a\nb", "`a b`"},
		{"", "` `"},
	}
	for _, tc := range cases {
		if got := codeSpan(tc.in); got != tc.want {
			t.Errorf("codeSpan(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRenderEntity_SubtypeHierarchy checks that a child names its supertype and
// a parent lists its subtypes.
func TestRenderEntity_SubtypeHierarchy(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"PaymentMethod": {Definition: "A way to pay."},
		"Card":          {Definition: "A card.", SubtypeOf: "PaymentMethod"},
		"BankTransfer":  {Definition: "A transfer.", SubtypeOf: "PaymentMethod"},
	}}
	got := render(m)
	if !strings.Contains(got, "**Subtype of** `PaymentMethod`") {
		t.Errorf("expected child to name its supertype:\n%s", got)
	}
	// names render alphabetically, so BankTransfer precedes Card.
	if !strings.Contains(got, "**Subtypes** — `BankTransfer`, `Card`") {
		t.Errorf("expected parent to list its subtypes:\n%s", got)
	}
}

// TestProse_EscapesHTMLOutsideCodeSpans pins the escaping rule ADR-0014
// records, case by case: angle brackets become character references outside a
// literal region and are left alone inside one, an ampersand is escaped only
// where it introduces a character reference, and Markdown is never touched.
//
// The "block" cases are the fields emitted as their own block — a description,
// a definition — where a code fence or an indented block is literal. Everything
// else lands inside a line, where it is not.
func TestProse_EscapesHTMLOutsideCodeSpans(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, in, want string
		block          bool
	}{
		{name: "plain text", in: "A parking ticket.", want: "A parking ticket."},
		{name: "tag", in: "an <img src=q onerror=alert(1)> tag", want: "an &lt;img src=q onerror=alert(1)&gt; tag"},
		{name: "markdown survives", in: "a `Ticket` with *emphasis*", want: "a `Ticket` with *emphasis*"},
		{name: "inside a code span", in: "a `<x>` span", want: "a `<x>` span"},
		{name: "in and out", in: "<a> `<b>` <c>", want: "&lt;a&gt; `<b>` &lt;c&gt;"},
		{name: "unclosed span is text", in: "a ` <b> tail", want: "a ` &lt;b&gt; tail"},
		{name: "double fence", in: "``a `<b>` c`` <d>", want: "``a `<b>` c`` &lt;d&gt;"},
		// An "&" is markup to nobody, so every one of these is left as the
		// author typed it. A character reference renders as the character it
		// names, which is what the author asked for; it can never become a tag,
		// because a parser decodes it to text and re-escapes it on output.
		{name: "bare ampersand", in: "R&D and a & b", want: "R&D and a & b"},
		{name: "named reference", in: "&lt;pre&gt;", want: "&lt;pre&gt;"},
		{name: "decimal reference", in: "&#60;", want: "&#60;"},
		{name: "hex reference", in: "&#x3c;", want: "&#x3c;"},
		{name: "reference-shaped but unterminated", in: "&lt and &amp", want: "&lt and &amp"},
		{name: "reference inside a code span", in: "`&lt;`", want: "`&lt;`"},
		// An angle bracket that opens no tag is not markup and is not touched.
		// A Markdown parser renders it as the character it is.
		{name: "comparison", in: "1 < 2 && 3 > 2", want: "1 < 2 && 3 > 2"},
		{name: "conceptual type", in: "map<string, int>", want: "map<string, int>"},
		{name: "multibyte passes through", in: "café — naïve <x>", want: "café — naïve &lt;x&gt;"},
		// An "&" inside a tag is escaped with it, so the whole tag reaches the
		// page as the bytes the author typed.
		{name: "ampersand inside a tag", in: `<a href="?a=1&amp;b=2">`, want: `&lt;a href="?a=1&amp;amp;b=2"&gt;`},

		// Markdown structure is never escaped. Escaping it was the previous
		// rule's undoing: a ">" turned into "&gt;" stops opening a blockquote,
		// so the indented code block inside collapsed into a paragraph and
		// shipped its contents live (#37 H2).
		{
			name:  "blockquote marker survives",
			block: true,
			in:    "Quoted from the migration notes:\n\n>     <script>alert(31)</script>\n\nNothing after is affected.",
			want:  "Quoted from the migration notes:\n\n>     <script>alert(31)</script>\n\nNothing after is affected.",
		},
		{name: "angle-bracket link destination", in: "[click](<http://example.com/a>)", want: "[click](<http://example.com/a>)"},
		{name: "autolink", in: "see <https://example.com> ok", want: "see <https://example.com> ok"},

		// An HTML block opens on a tag that is not a complete one, and a value
		// on a line can still begin a block: a scenario step is rendered as
		// "1. " and then the step, so the step opens the list item's first
		// block. Read only as inline content, "</div " is text and reaches the
		// page live. Both readings are taken and either one is enough to escape.
		{name: "unterminated closing tag", in: "</div ", want: "&lt;/div "},
		{name: "unterminated open tag", in: "<pre>x", want: "&lt;pre&gt;x"},

		// A backslash-escaped backtick is a literal character and opens
		// nothing. The scanner this replaced read every backtick as a
		// delimiter, so the tag between two of them shipped live (#37 F1).
		{
			name: "escaped backtick opens no span",
			in:   `Escaped tick: \` + "`" + ` then <b>bold</b> then \` + "`" + `.`,
			want: `Escaped tick: \` + "`" + ` then &lt;b&gt;bold&lt;/b&gt; then \` + "`" + `.`,
		},
		// A code span cannot cross a paragraph break, so neither backtick
		// opens one and the tag between them is ordinary inline HTML (#37 F2).
		{
			name:  "span cannot cross a paragraph break",
			block: true,
			in:    "A trailing ` marks it.\n\nLegacy importers emit <img src=x onerror=alert(1)>.\n\nA second ` closes nothing.",
			want:  "A trailing ` marks it.\n\nLegacy importers emit &lt;img src=x onerror=alert(1)&gt;.\n\nA second ` closes nothing.",
		},
		// Block-level code is literal in a block field: escaping there puts a
		// visible "&lt;" on the page, the fidelity bug ADR-0014 exists to
		// avoid. A backtick fence survived the old scanner only by accident
		// (#37 F3).
		{
			name:  "tilde fence is verbatim",
			block: true,
			in:    "Shape:\n\n~~~\n<policy enabled=true>\n~~~",
			want:  "Shape:\n\n~~~\n<policy enabled=true>\n~~~",
		},
		{
			name:  "indented code block is verbatim",
			block: true,
			in:    "Shape:\n\n    <policy enabled=true>",
			want:  "Shape:\n\n    <policy enabled=true>",
		},
		{
			name:  "backtick fence is verbatim",
			block: true,
			in:    "Shape:\n\n```\n<policy enabled=true>\n```",
			want:  "Shape:\n\n```\n<policy enabled=true>\n```",
		},
		// Inside a line there is no block context to open, so what looks like
		// a fence is text and the tag behind it is escaped.
		{name: "fence opener inside a line", in: "```<b>", want: "```&lt;b&gt;"},
		{name: "code fence info is not text", block: true, in: "```<b>\nx\n```", want: "```&lt;b&gt;\nx\n```"},
	}
	for _, tc := range cases {
		got := escapeProse(tc.in, tc.block)
		if got != tc.want {
			t.Errorf("%s: escapeProse(%q, %t) = %q, want %q", tc.name, tc.in, tc.block, got, tc.want)
		}
	}
}

// TestADR_0014_NoRawHTMLSurvivesRender is the whole-document form of the rule:
// take a model whose every prose field carries a payload that defeated the
// hand-rolled scanner, render it, and ask an independent CommonMark parser
// whether any raw HTML is left. This is the guard the old one should have been
// — it shared the scanner's model of Markdown, so it agreed with the bug.
func TestADR_0014_NoRawHTMLSurvivesRender(t *testing.T) {
	t.Parallel()

	const tag = "<img src=x onerror=alert(1)>"
	esc := "`"
	cases := []struct{ name, payload string }{
		{"plain tag", "Legacy importers emit " + tag + " in the notes."},
		// #37 F1: the escaped backtick opened a span the parser never sees.
		{"escaped backtick", `A trailing \` + esc + ` marks it, then ` + tag + `, then \` + esc + `.`},
		// #37 F2: two bare backticks separated by a paragraph break.
		{"paragraph break", "A trailing " + esc + " marks it.\n\n" + tag + " in the notes.\n\nA second " + esc + " here."},
		{"unclosed span", "A trailing " + esc + " then " + tag + "."},
		{"reference-encoded tag", "&#60;img src=x onerror=alert(1)&#62;"},
		{"html block", "<div onclick=alert(1)>\ntext\n</div>"},
		// An HTML block opens on a tag that never completes, and a scenario step
		// begins its list item's first block — so this is a block opener in one
		// position and inline text in another.
		{"unterminated closing tag", "</div "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &model.Model{
				Description: tc.payload,
				Glossary:    map[string]string{"Term": tc.payload},
				Enums: map[string]model.Enum{
					"Status": {Description: tc.payload, Values: []model.EnumValue{{Name: "active", Definition: tc.payload}}},
				},
				Entities: map[string]model.Entity{
					"Ticket": {
						Definition: tc.payload,
						Derived:    true,
						Derivation: tc.payload,
						Attributes: []model.Attribute{{Name: "rate", Type: "string", Description: tc.payload}},
						Actions:    []model.Action{{Name: "pay", Description: tc.payload}},
						Invariants: []model.Invariant{{ID: "paid", Statement: tc.payload}},
					},
				},
				Scenarios: []model.Scenario{{Name: tc.payload, Description: tc.payload, Steps: []string{tc.payload}}},
			}
			got := render(m)
			if html := rawHTML(t, got); len(html) > 0 {
				t.Errorf("raw HTML survived the render: %q\n%s", html, got)
			}
		})
	}
}

// TestADR_0014_BlockCodeStaysVerbatim is the other half of ADR-0014: a code
// block in a block-level field must reach the page as the author wrote it. The
// old pass escaped inside a "~~~" fence and an indented block, putting the
// visible "&lt;" on the page the rule exists to avoid (#37 F3).
func TestADR_0014_BlockCodeStaysVerbatim(t *testing.T) {
	t.Parallel()

	const payload = "Shape:\n\n~~~\n<policy enabled=true>\n~~~\n\nAnd indented:\n\n    <policy enabled=false>"
	m := &model.Model{
		Entities: map[string]model.Entity{"Ticket": {Definition: payload}},
	}
	got := render(m)

	if strings.Contains(got, "&lt;policy") {
		t.Errorf("escaped inside a code block, so the reader sees the escape:\n%s", got)
	}
	if !strings.Contains(got, "<policy enabled=true>") || !strings.Contains(got, "<policy enabled=false>") {
		t.Errorf("a code block did not survive verbatim:\n%s", got)
	}
	// Verbatim is only safe because a real parser agrees it is code.
	if html := rawHTML(t, got); len(html) > 0 {
		t.Errorf("the code block reached the page as live HTML %q:\n%s", html, got)
	}
}

// TestADR_0014_ProseRendersHTMLAsText walks every prose-bearing field through a
// render and checks that raw HTML lands as visible text rather than as markup,
// while the Markdown those fields are written in still works. Before the fix
// each of these strings reached the document as live HTML.
func TestADR_0014_ProseRendersHTMLAsText(t *testing.T) {
	t.Parallel()

	const tag = "<img src=q onerror=alert(1)>"
	m := &model.Model{
		Title:       "Title " + tag,
		Description: "Description " + tag,
		Glossary:    map[string]string{"Attendant": "Glossary " + tag},
		Enums: map[string]model.Enum{
			"Status": {
				Description: "Enum " + tag,
				Values:      []model.EnumValue{{Name: "active", Definition: "Value " + tag}},
			},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "Definition " + tag + " with a `code <span>` and *emphasis*",
				Derived:    true,
				Derivation: "Derivation " + tag,
				Relationships: []model.Relationship{
					{Entity: "Policy", Cardinality: "1:n", Role: "Role " + tag, Note: "Note " + tag},
				},
				Attributes: []model.Attribute{
					{Name: "paid", Type: "map<string, int>", Description: "Attribute " + tag},
				},
				Actions: []model.Action{
					{Name: "pay", Actor: "Attendant", Preserves: []string{"pre-" + tag}, Description: "Action " + tag},
				},
				Invariants: []model.Invariant{{ID: "ticket-paid", Statement: "Invariant " + tag}},
			},
			"Policy": {Definition: "A policy."},
		},
		Invariants: []model.Invariant{{ID: "model-rule", Statement: "Model invariant " + tag}},
		Scenarios: []model.Scenario{{
			Name:              "Scenario " + tag,
			Description:       "Scenario description " + tag,
			Actors:            []string{"Actor " + tag},
			Steps:             []string{"Step " + tag},
			InvariantsTouched: []string{"ticket-paid", "touched-" + tag},
		}},
	}
	got := render(m)

	const escaped = "&lt;img src=q onerror=alert(1)&gt;"
	// One occurrence per field above, plus the two invariant statements repeated
	// under "Invariants touched" — every one of them escaped.
	for _, field := range []string{
		"Title", "Description", "Glossary", "Enum", "Value", "Definition",
		"Derivation", "Role", "Note", "Attribute", "Action", "Invariant",
		"Model invariant", "Scenario", "Scenario description", "Actor", "Step",
		"pre-", "touched-",
	} {
		if !strings.Contains(got, field+" "+escaped) && !strings.Contains(got, field+escaped) {
			t.Errorf("%s field did not render its tag as text:\n%s", field, got)
		}
	}
	if strings.Contains(got, tag) {
		t.Errorf("a raw tag survived into the document:\n%s", got)
	}
	// The type column is prose too, and a conceptual type may hold angle
	// brackets legitimately. "<string," opens no tag, so it is not markup and
	// reaches the cell as the author wrote it — the rawHTML assertion below is
	// what makes that safe rather than merely convenient.
	if !strings.Contains(got, "| map<string, int> |") {
		t.Errorf("expected the conceptual type verbatim:\n%s", got)
	}
	if html := rawHTML(t, got); len(html) > 0 {
		t.Errorf("raw HTML survived the render: %q\n%s", html, got)
	}
	// Markdown in prose is the point of not escaping everything: the code span
	// keeps its angle brackets literal and the emphasis still renders.
	if !strings.Contains(got, "a `code <span>` and *emphasis*") {
		t.Errorf("expected Markdown and its code span to survive intact:\n%s", got)
	}
}

// TestRender_UnnormalisedProseDoesNotPanic pins the guard in parseSource.
// goldmark measures a line's indent in columns and its length in bytes, so a
// final line a tab can outrun is taken for a blank one and the fenced-code-block
// parser indexes it at the -1 that marks one. Every input here lints clean and
// crashed `modelith render` with a Go stack trace before the trailing newline
// went in; the first is upstream yuin/goldmark#556's own repro, which v1.8.4
// still panics on despite carrying the fix for it.
//
// Rendering has to stay sane, not merely survive: the payloads come back as the
// author typed them, and no raw HTML rides in on the workaround.
func TestRender_UnnormalisedProseDoesNotPanic(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, payload string }{
		{"upstream 556 repro", "*\n\t* \t~"},
		{"blockquote tab backtick", "A ticket.\n\n> \t`"},
		{"bare blockquote tab backtick", "> \t`"},
		{"blockquote tab tilde", "> \t~"},
		{"tab before a fence", "Shape:\n\n> \t```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &model.Model{
				Description: tc.payload,
				Entities: map[string]model.Entity{
					"Ticket": {Definition: tc.payload},
				},
				Scenarios: []model.Scenario{{Name: "Pay", Description: tc.payload}},
			}
			got := render(m)
			if !strings.Contains(got, tc.payload) {
				t.Errorf("payload %q did not reach the page verbatim:\n%s", tc.payload, got)
			}
			if html := rawHTML(t, got); len(html) > 0 {
				t.Errorf("raw HTML survived the render: %q\n%s", html, got)
			}
		})
	}
}

// TestADR_0014_AssembledLineEscapesAsOneLine is the regression for the
// cross-field pairing in #37: a `role:` and a `note:` are separate schema
// fields that share one rendered line, so escaping them separately let a stray
// backtick in the role pair with the note's backticks and leave the note's tag
// outside every code span, live. Whether a span is open is a property of the
// finished line, so the line is what gets escaped.
func TestADR_0014_AssembledLineEscapesAsOneLine(t *testing.T) {
	t.Parallel()

	const tag = "<img src=q onerror=alert(1)>"
	m := &model.Model{
		Glossary: map[string]string{"Term": "a ` stray then `" + tag + "` here"},
		Entities: map[string]model.Entity{
			"Team": {
				Definition: "A team.",
				Relationships: []model.Relationship{
					{Entity: "Member", Cardinality: "n:n", Role: "`Owner or `Member`", Note: "Rendered as `" + tag + "` in the UI"},
				},
				Actions: []model.Action{
					{Name: "add", Preserves: []string{"a ` stray"}, Description: "Shown as `" + tag + "` there"},
				},
				Invariants: []model.Invariant{{ID: "one ` tick", Statement: "Holds for `" + tag + "` rows"}},
			},
			"Member": {Definition: "A member."},
		},
		Scenarios: []model.Scenario{{
			Name:   "Join",
			Actors: []string{"a ` stray", "the `" + tag + "` operator"},
		}},
	}
	got := render(m)

	if html := rawHTML(t, got); len(html) > 0 {
		t.Errorf("a field paired with its neighbour and shipped live HTML %q:\n%s", html, got)
	}
	if bare := textOutsideCode(t, got); strings.Contains(bare, "<img") {
		t.Errorf("a tag escaped its code span into the document text:\n%s", bare)
	}
	if strings.Contains(got, tag) {
		t.Errorf("a raw tag survived into the document:\n%s", got)
	}
}

// TestRenderCodeSpans_HostileNamesStayInsideTheirSpan is the regression for
// issue #35: a backtick in an unconstrained field closed its code span early
// and let the remainder land as live Markdown. Every one of these fields is a
// bare string in the schema, so only the renderer can hold the line.
func TestRenderCodeSpans_HostileNamesStayInsideTheirSpan(t *testing.T) {
	t.Parallel()

	const breakout = "a ` <img src=q onerror=alert(1)> `"
	m := &model.Model{
		Enums: map[string]model.Enum{
			"Status": {Values: []model.EnumValue{{Name: breakout}, {Name: "pipe|value"}}},
		},
		Entities: map[string]model.Entity{
			"Ticket": {
				Definition: "A stub.",
				Attributes: []model.Attribute{{Name: breakout, Type: "string"}, {Name: "pipe|name", Type: "string"}},
				Actions: []model.Action{
					{Name: breakout},
					{Name: "structured " + breakout, Actor: "actor " + breakout},
				},
			},
		},
	}
	got := render(m)

	// The fence widens to two backticks and the value is padded, so the whole
	// string stays inside the span (CommonMark strips the padding on render).
	const span = "`` a ` <img src=q onerror=alert(1)> ` ``"
	for _, want := range []string{
		"| " + span + " |",                        // enum value name
		"| " + span + " | string |",               // attribute name
		"- `` structured a ` <img",                // structured action name
		"actor `` actor a ` <img",                 // action actor
		"- `` a ` <img",                           // bare action name, detailed list
		"| `pipe\\|value` |", "| `pipe\\|name` |", // a pipe cannot end the cell
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	// The inline "**Actions:**" line is a separate call site, reached only when
	// no action carries structure.
	inline := render(&model.Model{Entities: map[string]model.Entity{
		"Ticket": {Definition: "A stub.", Actions: []model.Action{{Name: breakout}}},
	}})
	if want := "**Actions:** " + span + "\n"; !strings.Contains(inline, want) {
		t.Errorf("expected %q in:\n%s", want, inline)
	}

	// Nothing hostile survives outside a code span, and nothing reaches the
	// page as markup.
	if html := rawHTML(t, got); len(html) > 0 {
		t.Errorf("a name reached the page as live HTML %q:\n%s", html, got)
	}
	bare := textOutsideCode(t, got)
	for _, tok := range []string{"<img", "onerror"} {
		if strings.Contains(bare, tok) {
			t.Errorf("%q escaped its code span into the document text:\n%s", tok, bare)
		}
	}
	// A pipe that broke out of its cell changes a row's cell count. The enum
	// table is two columns and the attribute table three, header rows included.
	if want := []int{2, 2, 2, 3, 3, 3}; !slices.Equal(tableRows(t, got), want) {
		t.Errorf("table row widths = %v, want %v — a pipe escaped its cell:\n%s", tableRows(t, got), want, got)
	}
}
