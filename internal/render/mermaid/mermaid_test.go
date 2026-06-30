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
			"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: card}}},
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

func TestERLabelPrecedenceAndSanitize(t *testing.T) {
	// role wins over ownership wins over cardinality; backticks/brackets/newlines
	// are neutralized so the quoted label stays valid Mermaid.
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

func TestERLabelFallsBackToOwnershipThenCardinality(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"A": {Definition: "a", Relationships: []model.Relationship{{Entity: "B", Cardinality: "1:n", Ownership: "owned"}}},
		"B": {Definition: "b", Relationships: []model.Relationship{{Entity: "A", Cardinality: "n:1"}}},
	}}
	out := ER(m)
	if !strings.Contains(out, `: "owned"`) {
		t.Errorf("expected ownership label when role is absent; got:\n%s", out)
	}
	if !strings.Contains(out, `: "n:1"`) {
		t.Errorf("expected cardinality label when role and ownership are absent; got:\n%s", out)
	}
}
