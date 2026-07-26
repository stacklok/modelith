package mermaid

import (
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/model"
)

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

// TestERFoldsOneReciprocalPerEdge guards the fold against absorbing more than
// one declaration from the opposite end. Three declarations on one pair are not
// one relationship, and the linter deliberately leaves such a pair alone, so the
// renderer must not merge them all into a single line.
func TestERFoldsOneReciprocalPerEdge(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Alpha": {Definition: "one", Relationships: []model.Relationship{
			{Entity: "Beta", Cardinality: "n:n", Role: "Owner"},
		}},
		"Beta": {Definition: "two", Relationships: []model.Relationship{
			{Entity: "Alpha", Cardinality: "n:n", Role: "Member"},
			{Entity: "Alpha", Cardinality: "n:n", Role: "Watcher"},
		}},
	}}
	out := ER(m)
	if n := strings.Count(out, " : "); n != 2 {
		t.Errorf("expected two edges, got %d:\n%s", n, out)
	}
	for _, want := range []string{"    Alpha }o..o{ Beta : \"Owner\"\n", "    Beta }o..o{ Alpha : \"Watcher\"\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected edge %q; got:\n%s", want, out)
		}
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
