package mermaid

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/model"
)

// firstDiff reports the first line where want and got differ, so a golden
// failure points at the change instead of just saying "they differ." Mirrors
// internal/render/markdown's helper of the same name.
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

// assertGolden renders m and compares it byte-for-byte against the named file
// under testdata/, failing with the first differing line on mismatch.
func assertGolden(t *testing.T, m *model.Model, goldenPath string) {
	t.Helper()
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	got := ER(m)
	if got != string(want) {
		t.Errorf("rendered output does not match %s.\n%s", goldenPath, firstDiff(string(want), got))
	}
}

func TestERDeclaresAllEntities(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"Alpha": {Definition: "a"},
		"Beta":  {Definition: "b"}, // unconnected — must still appear as a node
	}}
	out := ER(m)
	if !strings.HasPrefix(out, "erDiagram\n") {
		t.Fatalf("expected erDiagram header, got:\n%s", out)
	}
	for _, name := range []string{"Alpha", "Beta"} {
		if !strings.Contains(out, "    "+name+" {}") {
			t.Errorf("expected entity %q declared as a node; got:\n%s", name, out)
		}
	}
}

// TestERCardinalityNotation covers the crow's-foot glyphs. Every case declares
// ownership so the connector stays solid ("--") and the assertion is about the
// end markers alone; the connector itself is TestADR_0008_OwnershipIsLineStyle.
func TestERCardinalityNotation(t *testing.T) {
	cases := map[string]string{
		"1:1":     "||--||",
		"1:n":     "||--o{",
		"n:1":     "}o--||",
		"n:n":     "}o--o{",
		"unknown": "}o--o{", // unrecognized cardinality falls back to many-to-many
	}
	for card, want := range cases {
		m := &model.Model{Entities: map[string]model.Entity{
			"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: card, Ownership: "owned"}}},
			"B": {Definition: "b"},
		}}
		out := ER(m)
		if !strings.Contains(out, want) {
			t.Errorf("cardinality %q: expected notation %q in:\n%s", card, want, out)
		}
	}
}

func TestERDedupesReciprocalEdges(t *testing.T) {
	// The same pair + same label declared from both sides should emit one edge.
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "n:n", Role: "peer"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "A", Cardinality: "n:n", Role: "peer"}}},
	}}
	out := ER(m)
	if n := strings.Count(out, `: "peer"`); n != 1 {
		t.Errorf("expected reciprocal edge deduped to 1, got %d:\n%s", n, out)
	}
}

func TestERDedupesInverseCardinality(t *testing.T) {
	// Declared from both sides with inverse cardinalities (A "1:n" B, B "n:1" A)
	// and the same label: that's one relationship seen from two ends, so it must
	// collapse to a single edge despite the differing raw cardinality strings.
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Role: "owns"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "A", Cardinality: "n:1", Role: "owns"}}},
	}}
	out := ER(m)
	if n := strings.Count(out, `: "owns"`); n != 1 {
		t.Errorf("expected inverse-cardinality reciprocal deduped to 1 edge, got %d:\n%s", n, out)
	}
}

func TestERRendersConflictingReciprocalEdges(t *testing.T) {
	// Contradictory reciprocal cardinalities (A "1:n" B, B "1:1" A) are NOT the
	// same relationship inverted, so both edges render — the conflict stays
	// visible in the diagram rather than one side silently winning. (`modelith lint`
	// reports it as an error.)
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Role: "x"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "A", Cardinality: "1:1", Role: "x"}}},
	}}
	out := ER(m)
	if n := strings.Count(out, `: "x"`); n != 2 {
		t.Errorf("expected conflicting reciprocal edges to both render (2), got %d:\n%s", n, out)
	}
}

func TestERLabelSanitizesRole(t *testing.T) {
	// backticks/brackets/newlines are neutralized so the quoted label stays
	// valid Mermaid.
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{
			{Entity: "B", Cardinality: "1:n", Role: "`Owner` [x]\nmore", Ownership: "owned"},
		}},
		"B": {Definition: "b"},
	}}
	out := ER(m)
	if !strings.Contains(out, `: "Owner (x) more"`) {
		t.Errorf("expected sanitized role label `Owner (x) more`; got:\n%s", out)
	}
}

// TestADR_0008_LabelIsRoleOrEmpty pins the label half of ADR-0008: the role is
// the only thing that labels a line. Ownership rides on the line style and the
// precise cardinality lives in the Markdown table, so neither is a fallback.
func TestADR_0008_LabelIsRoleOrEmpty(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Ownership: "owned"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "C", Cardinality: "n:1"}}},
		"C": {Definition: "c"},
	}}
	out := ER(m)
	for _, want := range []string{"    A ||--o{ B : \"\"\n", "    B }o..|| C : \"\"\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected empty label line %q; got:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{`"owned"`, `"referenced"`, `"1:n"`, `"n:1"`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("expected no %s label; got:\n%s", unwanted, out)
		}
	}
}

// TestADR_0008_OwnershipIsLineStyle pins the connector half of ADR-0008: owned
// draws an identifying (solid) line, referenced and omitted a non-identifying
// (dashed) one.
func TestADR_0008_OwnershipIsLineStyle(t *testing.T) {
	cases := map[string]string{
		"owned":      "    A ||--o{ B : \"\"\n",
		"referenced": "    A ||..o{ B : \"\"\n",
		"":           "    A ||..o{ B : \"\"\n", // omitted defaults to referenced
	}
	for ownership, want := range cases {
		m := &model.Model{Entities: map[string]model.Entity{
			"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Ownership: ownership}}},
			"B": {Definition: "b"},
		}}
		if out := ER(m); !strings.Contains(out, want) {
			t.Errorf("ownership %q: expected %q in:\n%s", ownership, want, out)
		}
	}
}

// TestADR_0008_OwnershipFoldsAcrossDeclarations pins the dedupe half of
// ADR-0008: ownership belongs to the relationship, not to the end that declared
// it, so a parent's `owned` and the child's `referenced` fold into one solid
// edge rather than a contradictory solid-plus-dashed pair.
func TestADR_0008_OwnershipFoldsAcrossDeclarations(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Parent": {Definition: "p", Relationships: []model.Relationship{{Entity: "Child", Cardinality: "1:n", Ownership: "owned"}}},
		"Child":  {Definition: "c", Relationships: []model.Relationship{{Entity: "Parent", Cardinality: "n:1", Ownership: "referenced"}}},
	}}
	out := ER(m)
	if want := "    Child }o--|| Parent : \"\"\n"; !strings.Contains(out, want) {
		t.Errorf("expected folded identifying edge %q; got:\n%s", want, out)
	}
	if n := strings.Count(out, " Parent : "); n != 1 {
		t.Errorf("expected one edge between the pair, got %d:\n%s", n, out)
	}
}

// TestADR_0008_FoldsOnlyGenuineReciprocals pins the fold predicate of ADR-0008.
// Two declarations become one line only when they are one relationship seen
// from two sides — opposite ends, inverse cardinality, and at most one end
// claiming `owned` — or when one is an exact duplicate of the other. Roles are
// not part of the predicate: the two ends of a composition naturally name
// different roles. Everything else draws both lines, so no declaration
// vanishes.
func TestADR_0008_FoldsOnlyGenuineReciprocals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		a, b  model.Relationship // a declared by A; b declared by B unless bFromA
		bFrom string             // entity that declares b: "A" or "B"
		want  []string
	}{
		{
			name:  "opposite ends, one owned: one solid line",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned"},
			b:     model.Relationship{Entity: "A", Cardinality: "n:1", Ownership: "referenced"},
			bFrom: "B",
			want:  []string{"    A ||--o{ B : \"\"\n"},
		},
		{
			name:  "opposite ends, neither owned: one dashed line",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n"},
			b:     model.Relationship{Entity: "A", Cardinality: "n:1"},
			bFrom: "B",
			want:  []string{"    A ||..o{ B : \"\"\n"},
		},
		{
			name:  "opposite ends, both owned: a contradiction, so both lines draw",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned"},
			b:     model.Relationship{Entity: "A", Cardinality: "n:1", Ownership: "owned"},
			bFrom: "B",
			want:  []string{"    A ||--o{ B : \"\"\n", "    B }o--|| A : \"\"\n"},
		},
		{
			name:  "same end, ownership differs: two relationships, so both lines draw",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned"},
			b:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "referenced"},
			bFrom: "A",
			want:  []string{"    A ||--o{ B : \"\"\n", "    A ||..o{ B : \"\"\n"},
		},
		{
			name:  "same end, identical: an exact duplicate draws once",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned"},
			b:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned"},
			bFrom: "A",
			want:  []string{"    A ||--o{ B : \"\"\n"},
		},
		{
			name:  "opposite ends, roles differ: one line, labelled by the owning end",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Ownership: "owned", Role: "part"},
			b:     model.Relationship{Entity: "A", Cardinality: "n:1", Role: "whole"},
			bFrom: "B",
			want:  []string{"    A ||--o{ B : \"part\"\n"},
		},
		{
			name:  "opposite ends, roles differ, neither owns: the first entity's role labels it",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Role: "part"},
			b:     model.Relationship{Entity: "A", Cardinality: "n:1", Role: "whole"},
			bFrom: "B",
			want:  []string{"    A ||..o{ B : \"part\"\n"},
		},
		{
			name:  "same end, roles differ: an Owner and a Member are two relationships",
			a:     model.Relationship{Entity: "B", Cardinality: "1:n", Role: "Owner"},
			b:     model.Relationship{Entity: "B", Cardinality: "1:n", Role: "Member"},
			bFrom: "A",
			want:  []string{"    A ||..o{ B : \"Owner\"\n", "    A ||..o{ B : \"Member\"\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := model.Entity{Definition: "a", Relationships: []model.Relationship{tc.a}}
			b := model.Entity{Definition: "b"}
			if tc.bFrom == "A" {
				a.Relationships = append(a.Relationships, tc.b)
			} else {
				b.Relationships = []model.Relationship{tc.b}
			}
			out := ER(&model.Model{Entities: map[string]model.Entity{"A": a, "B": b}})
			if n := strings.Count(out, " : "); n != len(tc.want) {
				t.Errorf("expected %d edge(s), got %d:\n%s", len(tc.want), n, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("expected edge %q; got:\n%s", want, out)
				}
			}
		})
	}
}

// TestADR_0008_ReciprocalCompositionFoldsToOneEdge is the regression guard for
// the textbook composition pattern: a parent owns a child and the child
// references the parent back, each naming its own end's role. That is one
// relationship seen from two sides — the case the fold exists for — so it must
// draw exactly one solid line, labelled by the owning end. Requiring the two
// roles to match instead would split it into a solid line and a dashed one.
// `TestADR_0008_ReciprocalCompositionLintsClean` pins the linter's half.
func TestADR_0008_ReciprocalCompositionFoldsToOneEdge(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Alpha": {Definition: "one", Relationships: []model.Relationship{
			{Entity: "Beta", Cardinality: "1:n", Ownership: "owned", Role: "part"},
		}},
		"Beta": {Definition: "two", Relationships: []model.Relationship{
			{Entity: "Alpha", Cardinality: "n:1", Ownership: "referenced", Role: "whole"},
		}},
	}}
	out := ER(m)
	if want := "    Alpha ||--o{ Beta : \"part\"\n"; !strings.Contains(out, want) {
		t.Errorf("expected one solid edge %q; got:\n%s", want, out)
	}
	if n := strings.Count(out, " : "); n != 1 {
		t.Errorf("expected exactly one edge, got %d:\n%s", n, out)
	}
}

// edgeLines returns the rendered relationship lines, sorted. Sorting drops the
// one thing about the output that legitimately follows the source: the order
// the author wrote the declarations in. What must not vary is the set of lines.
func edgeLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, " : ") {
			lines = append(lines, l)
		}
	}
	sort.Strings(lines)
	return lines
}

// TestADR_0008_AmbiguousPairingKeepsEveryDeclaration is the regression guard for
// the defect ADR-0008's earlier "no declaration disappears" wording promised but
// did not deliver. When one end declares the same line twice and the other
// declares it once, no pairing exists in the format: the single declaration is
// the reciprocal of one of the two, and nothing says which. Folding it into
// either drops the other's role, and which one depends on declaration order.
// So nothing folds — every declaration draws.
func TestADR_0008_AmbiguousPairingKeepsEveryDeclaration(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Project": {Definition: "a container", Relationships: []model.Relationship{
			{Entity: "Policy", Cardinality: "1:n", Role: "defaults", Ownership: "referenced"},
			{Entity: "Policy", Cardinality: "1:n", Role: "overrides", Ownership: "referenced"},
		}},
		"Policy": {Definition: "a rule", Relationships: []model.Relationship{
			{Entity: "Project", Cardinality: "n:1", Role: "parent", Ownership: "owned"},
		}},
	}}
	out := ER(m)
	want := []string{
		`    Policy }o--|| Project : "parent"`,
		`    Project ||..o{ Policy : "defaults"`,
		`    Project ||..o{ Policy : "overrides"`,
	}
	if got := edgeLines(out); !slices.Equal(got, want) {
		t.Errorf("expected every declaration drawn:\n%v\ngot:\n%v\nin:\n%s", want, got, out)
	}
}

// TestERAmbiguousPairingIsOrderIndependent pins the property the round-3 defect
// broke: reordering an entity's relationship list must not change which
// declarations survive or how they draw. Only the order the lines are listed in
// follows the source, so the comparison is on the sorted set.
func TestERAmbiguousPairingIsOrderIndependent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		alpha, bet []model.Relationship
		swapAlpha  bool // swap alpha's two declarations rather than beta's
	}{
		{
			name: "two on the first end, one owned, one back",
			alpha: []model.Relationship{
				{Entity: "Beta", Cardinality: "1:n", Role: "P", Ownership: "owned"},
				{Entity: "Beta", Cardinality: "1:n", Role: "Q", Ownership: "referenced"},
			},
			bet:       []model.Relationship{{Entity: "Alpha", Cardinality: "n:1", Role: "R", Ownership: "referenced"}},
			swapAlpha: true,
		},
		{
			name: "two on the first end, the owning declaration coming back",
			alpha: []model.Relationship{
				{Entity: "Beta", Cardinality: "1:n", Role: "P", Ownership: "referenced"},
				{Entity: "Beta", Cardinality: "1:n", Role: "Q", Ownership: "referenced"},
			},
			bet:       []model.Relationship{{Entity: "Alpha", Cardinality: "n:1", Role: "R", Ownership: "owned"}},
			swapAlpha: true,
		},
		{
			name:  "one on the first end, two coming back",
			alpha: []model.Relationship{{Entity: "Beta", Cardinality: "1:n", Role: "P", Ownership: "owned"}},
			bet: []model.Relationship{
				{Entity: "Alpha", Cardinality: "n:1", Role: "R1", Ownership: "referenced"},
				{Entity: "Alpha", Cardinality: "n:1", Role: "R2", Ownership: "referenced"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			build := func(swapped bool) string {
				alpha, bet := slices.Clone(tc.alpha), slices.Clone(tc.bet)
				if swapped {
					if tc.swapAlpha {
						alpha[0], alpha[1] = alpha[1], alpha[0]
					} else {
						bet[0], bet[1] = bet[1], bet[0]
					}
				}
				return ER(&model.Model{Entities: map[string]model.Entity{
					"Alpha": {Definition: "one", Relationships: alpha},
					"Beta":  {Definition: "two", Relationships: bet},
				}})
			}
			asDeclared, swapped := build(false), build(true)
			if got, want := edgeLines(swapped), edgeLines(asDeclared); !slices.Equal(got, want) {
				t.Errorf("swapping the declaration order changed the edges:\nas declared: %v\nswapped:     %v", want, got)
			}
			if n := len(edgeLines(asDeclared)); n != len(tc.alpha)+len(tc.bet) {
				t.Errorf("expected every declaration drawn (%d), got %d:\n%s", len(tc.alpha)+len(tc.bet), n, asDeclared)
			}
		})
	}
}

// TestADR_0008_SelfRelationshipRendersInEntityBlock pins the self-relationship
// half of ADR-0008: Mermaid's ER layout has no self-loop handling (issue #26),
// so the relationship becomes a row in the entity's own block and no edge.
func TestADR_0008_SelfRelationshipRendersInEntityBlock(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"Record": {Definition: "r", Relationships: []model.Relationship{
			{Entity: "Record", Cardinality: "1:0..1", Role: "`Predecessor`"},
			{Entity: "Note", Cardinality: "1:n", Ownership: "owned"},
		}},
		"Note": {Definition: "n"},
	}}
	out := ER(m)
	want := "    Record {\n        Record self \"1:0..1 — Predecessor\"\n    }\n"
	if !strings.Contains(out, want) {
		t.Errorf("expected self-relationship row %q; got:\n%s", want, out)
	}
	if strings.Contains(out, "Record ||--o| Record") || strings.Contains(out, "Record ||..o| Record") {
		t.Errorf("expected no self edge; got:\n%s", out)
	}
	if !strings.Contains(out, "    Note {}\n") {
		t.Errorf("expected an entity with no self-relationships to stay {}; got:\n%s", out)
	}
}

// TestADR_0008_SelfRowCarriesBothCardinalitySides pins the no-information-loss
// half of ADR-0008's in-box rows: the row replaces an edge whose two end
// markers encoded both sides of the cardinality, so the row shows both. The
// declaring side is exactly what a target-side-only row would drop — "1:n" and
// "0..5:1" would both collapse to their right-hand side alone.
func TestADR_0008_SelfRowCarriesBothCardinalitySides(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rel  model.Relationship
		want string
	}{
		{
			name: "cardinality only",
			rel:  model.Relationship{Entity: "Record", Cardinality: "1:n"},
			want: `Record self "1:n"`,
		},
		{
			name: "a bounded declaring side survives",
			rel:  model.Relationship{Entity: "Record", Cardinality: "0..5:1"},
			want: `Record self "0..5:1"`,
		},
		{
			name: "owned is spelled out, since there is no line to carry it",
			rel:  model.Relationship{Entity: "Record", Cardinality: "1:n", Ownership: "owned", Role: "`Part`"},
			want: `Record self "1:n owned — Part"`,
		},
		{
			name: "referenced stays implicit, matching the dashed default",
			rel:  model.Relationship{Entity: "Record", Cardinality: "1:0..1", Ownership: "referenced", Role: "Predecessor"},
			want: `Record self "1:0..1 — Predecessor"`,
		},
		{
			name: "a quote in the role cannot break out of the comment",
			rel:  model.Relationship{Entity: "Record", Cardinality: "n:n", Role: `a "quoted" [role]`},
			want: `Record self "n:n — a 'quoted' (role)"`,
		},
		{
			name: "a backslash in the role is dropped, not doubled by %q",
			rel:  model.Relationship{Entity: "Record", Cardinality: "n:n", Role: `a\b`},
			want: `Record self "n:n — ab"`,
		},
		{
			name: "a cardinality with no colon is shown whole",
			rel:  model.Relationship{Entity: "Record", Cardinality: "bogus"},
			want: `Record self "bogus"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &model.Model{Entities: map[string]model.Entity{
				"Record": {Definition: "r", Relationships: []model.Relationship{tc.rel}},
			}}
			if out := ER(m); !strings.Contains(out, tc.want) {
				t.Errorf("expected row %q in:\n%s", tc.want, out)
			}
		})
	}
}

// TestERMultipleSelfRelationships guards the row names: Mermaid does not
// disambiguate two attributes sharing a name, so each row gets its own.
func TestERMultipleSelfRelationships(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Record": {Definition: "r", Relationships: []model.Relationship{
			{Entity: "Record", Cardinality: "1:0..1", Role: "`Predecessor`"},
			{Entity: "Record", Cardinality: "1:n", Ownership: "owned", Role: "`Part`"},
			{Entity: "Record", Cardinality: "n:n", Role: "`Peer`"},
		}},
	}}
	want := "    Record {\n" +
		"        Record self \"1:0..1 — Predecessor\"\n" +
		"        Record self2 \"1:n owned — Part\"\n" +
		"        Record self3 \"n:n — Peer\"\n" +
		"    }\n"
	if out := ER(m); !strings.Contains(out, want) {
		t.Errorf("expected distinct self rows:\n%s\ngot:\n%s", want, out)
	}
}

// TestERSelfRelationshipsDedupe guards the row list against a declaration
// repeated verbatim: two rows that would read identically carry no more than
// one, and the numbering stays contiguous. Rows that differ in any rendered
// part are kept, since the reader can tell them apart.
func TestERSelfRelationshipsDedupe(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Record": {Definition: "r", Relationships: []model.Relationship{
			{Entity: "Record", Cardinality: "0..1:0..1", Role: "`Predecessor`"},
			{Entity: "Record", Cardinality: "0..1:0..1", Role: "`Predecessor`"},
			{Entity: "Record", Cardinality: "0..1:0..1", Ownership: "owned", Role: "`Predecessor`"},
		}},
	}}
	want := "    Record {\n" +
		"        Record self \"0..1:0..1 — Predecessor\"\n" +
		"        Record self2 \"0..1:0..1 owned — Predecessor\"\n" +
		"    }\n"
	if out := ER(m); !strings.Contains(out, want) {
		t.Errorf("expected deduped self rows:\n%s\ngot:\n%s", want, out)
	}
}

// TestADR_0003_BoundedCardinalityRendersNearestGlyph pins the render half of
// ADR-0003 and the capture-first principle in ADR-0002: Mermaid has no numeric
// bound, so exact and bounded counts render as the nearest crow's-foot glyph
// (the precise count lives in the Markdown, not the diagram). Ownership is
// declared so the connector stays solid and the glyphs read unchanged.
func TestADR_0003_BoundedCardinalityRendersNearestGlyph(t *testing.T) {
	cases := map[string]string{
		"1:2":    "||--|{", // exactly two -> one-or-many
		"1:1..n": "||--|{", // at least one -> one-or-many
		"1:0..1": "||--o|", // optional -> zero-or-one
		"2:1":    "}|--||", // exact on the left side (crow's-foot mirror)
	}
	for card, want := range cases {
		m := &model.Model{Entities: map[string]model.Entity{
			"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: card, Ownership: "owned"}}},
			"B": {Definition: "b"},
		}}
		if out := ER(m); !strings.Contains(out, want) {
			t.Errorf("cardinality %q: expected notation %q in:\n%s", card, want, out)
		}
	}
}

// TestERDedupesSemanticallyEqualInverses guards the review fix: a pair declared
// from both sides with equal-but-differently-written cardinalities collapses to
// one edge.
func TestERDedupesSemanticallyEqualInverses(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Role: "owns"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "A", Cardinality: "0..n:1", Role: "owns"}}},
	}}
	if n := strings.Count(ER(m), `: "owns"`); n != 1 {
		t.Errorf("expected semantically-equal inverse deduped to 1 edge, got %d:\n%s", n, ER(m))
	}
}

// TestERRole_HostileCharacters pins current output for a role that packs
// every hostile character sanitize knows about (backticks, quotes, brackets, a
// literal newline) alongside characters it does NOT neutralize: '<', '>', and
// '%%'. This golden pins the CURRENT (buggy) behavior on purpose, not the
// desired one: fixing issue #29 must update this golden as part of that fix,
// not treat the diff as a regression. This test pins only the generated `.mmd`
// source text produced by ER() — it does not verify how a Mermaid renderer
// interprets that text.
//
// The role also embeds a `%%{init: ...}%%`-shaped substring. Separately, a
// real render of that shape through mmdc was confirmed to parse successfully
// while silently changing the whole diagram's theme and dropping this edge's
// label text — a more severe manifestation of #29 than plain character
// passthrough into the label. This golden pins that the substring reaches the
// generated source unescaped; it does not itself exercise or assert the
// renderer-level effect, which was only checked by hand outside this test.
func TestERRole_HostileCharacters(t *testing.T) {
	t.Parallel()
	role := "he said \"hi\" [bracket] `tick` {brace} <angle> #hash %%pct %%{init: {'theme':'forest'}}%%\nsecond line"
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{
			{Entity: "B", Cardinality: "1:n", Role: role, Ownership: "owned"},
		}},
		"B": {Definition: "b"},
	}}
	assertGolden(t, m, "testdata/hostile_role.golden.mmd")
}

// TestERRole_LongUnbrokenWord pins current output for a very long role with no
// spaces to break on. The renderer has no wrapping logic, so it passes the
// word through whole; this guards that a long label doesn't get truncated,
// panic, or otherwise misrender.
func TestERRole_LongUnbrokenWord(t *testing.T) {
	t.Parallel()
	longWord := strings.Repeat("Loremipsumdolorsitametconsecteturadipiscingelit", 4) // 188 chars, no spaces
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{
			{Entity: "B", Cardinality: "1:n", Role: longWord, Ownership: "owned"},
		}},
		"B": {Definition: "b"},
	}}
	assertGolden(t, m, "testdata/long_word_role.golden.mmd")
}

// TestERSelfRow_EntityNamedSelfDoesNotCollide guards an entity literally named
// "Self": the self-row attribute names are always the fixed literal "self",
// "self2", … regardless of the entity's own name, so an entity named "Self"
// must not produce a doubled or otherwise confused row name.
func TestERSelfRow_EntityNamedSelfDoesNotCollide(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Self": {Definition: "s", Relationships: []model.Relationship{
			{Entity: "Self", Cardinality: "0..5:1", Role: "`Mirror`"},
			{Entity: "Self", Cardinality: "1:2", Role: "`Twin`", Ownership: "owned"},
		}},
	}}
	assertGolden(t, m, "testdata/self_named_entity.golden.mmd")
}

// TestER_UndeclaredRelationshipTarget pins that ER renders a well-formed edge
// to a relationship target that has no entry in Entities at all. ER does not
// validate references — that's modelith lint's job — so it must render
// without panicking, and the undeclared entity gets no {} block of its own
// since it never appears in EntityNames().
func TestER_UndeclaredRelationshipTarget(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Node": {Definition: "n", Relationships: []model.Relationship{
			{Entity: "Ghost", Cardinality: "1:n", Role: "haunts", Ownership: "owned"},
		}},
	}}
	assertGolden(t, m, "testdata/undeclared_target.golden.mmd")
}
