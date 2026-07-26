package markdown

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/model"
)

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
	got := Render(with)
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
	if strings.Contains(Render(without), "## Invariants") {
		t.Fatalf("did not expect an Invariants section when there are none:\n%s", Render(without))
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
	got := Render(withDerivation)
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
	got = Render(withoutDerivation)
	if !strings.Contains(got, "**Derived.** Computed on demand from other state; never persisted.\n\n") {
		t.Fatalf("expected a generic derived marker when derivation is omitted, got:\n%s", got)
	}

	notDerived := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {Definition: "A ranked view over current scores."},
		},
	}
	if strings.Contains(Render(notDerived), "**Derived") {
		t.Fatalf("did not expect a derived marker on a non-derived entity:\n%s", Render(notDerived))
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
	got := Render(m)
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
	got := Render(m)

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
	got := Render(m)

	for _, want := range []string{
		"## Imports\n",
		"- **`payments`** — [../payments/payments.modelith.yaml](../payments/payments.modelith.md)\n",
		"- **`billing`** — [./legacy/pay-v2.modelith.yaml](./legacy/pay-v2.modelith.md)\n",
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

	if strings.Contains(Render(&model.Model{Entities: m.Entities}), "## Imports") {
		t.Error("did not expect an Imports section when there are no imports")
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
	got := Render(m)

	for _, want := range []string{
		// The label is backslash-escaped plain text, so nothing in it opens a
		// tag or closes the label early.
		`- **` + "`payments`" + `** — [./p.modelith.yaml\) \<img src=x onerror=alert\(1\)\> \[click me\]\(\#](./p.modelith.yaml%29%20%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E%20%5Bclick%20me%5D%28%23.md)` + "\n",
		// A pipe survives in the destination as %7C rather than being escaped
		// for a table cell it is not in.
		"[./a\\|b.modelith.yaml](./a%7Cb.modelith.md)\n",
		"[./a b.modelith.yaml](./a%20b.modelith.md)\n",
		// Every control character is replaced before the value is written.
		"- **`line break`** — [./a \\#\\# Injected  b.modelith.yaml](./a%0A%23%23%20Injected%0A%0Ab.modelith.md)\n",
		"[./a\\`b\\`\\`c.modelith.yaml](./a%60b%60%60c.modelith.md)\n",
		// The deep link is assembled from escaped parts: escaping the finished
		// link instead would put a backslash inside the destination.
		"| `paidWith` | [payments.PaymentMethod](./p.modelith.yaml%29%20%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E%20%5Bclick%20me%5D%28%23.md#paymentmethod) |  |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	// Every occurrence of a breakout token is a backslash-escaped one.
	for _, tok := range []string{"<img", "[click me]"} {
		if strings.Count(got, tok) != strings.Count(got, `\`+tok) {
			t.Errorf("an unescaped %q survived in the imports section:\n%s", tok, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "#") && strings.Contains(line, "Injected") {
			t.Errorf("a path injected the heading %q:\n%s", line, got)
		}
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
	got := Render(m)
	if !strings.Contains(got, "**Subtype of** `PaymentMethod`") {
		t.Errorf("expected child to name its supertype:\n%s", got)
	}
	// names render alphabetically, so BankTransfer precedes Card.
	if !strings.Contains(got, "**Subtypes** — `BankTransfer`, `Card`") {
		t.Errorf("expected parent to list its subtypes:\n%s", got)
	}
}
