package model

import "testing"

func TestParseRejectsUnknownField(t *testing.T) {
	// UnmarshalStrict must reject keys that aren't in the model — catching typos
	// like `entites:` before they silently vanish.
	src := `
kind: DomainModel
version: v1
bogusField: nope
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatal("expected an error for an unknown top-level field, got nil")
	}
}

func TestParseRejectsMalformedYAML(t *testing.T) {
	if _, err := Parse([]byte("kind: [unclosed")); err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	}
}

func TestParseAcceptsValid(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A thing.
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Kind != "DomainModel" || len(m.Entities) != 1 {
		t.Fatalf("unexpected parse result: %+v", m)
	}
}

func TestParseActionStringAndObject(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A thing.
    actions:
      - create
      - name: archive
        actor: Owner
        preserves: [keep-one]
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	acts := m.Entities["Project"].Actions
	if len(acts) != 2 {
		t.Fatalf("expected 2 actions, got %d: %+v", len(acts), acts)
	}
	if acts[0].Name != "create" || acts[0].Actor != "" {
		t.Fatalf("bare action should parse to Name only, got %+v", acts[0])
	}
	if acts[1].Name != "archive" || acts[1].Actor != "Owner" || len(acts[1].Preserves) != 1 {
		t.Fatalf("structured action mis-parsed: %+v", acts[1])
	}
}

func TestParseActionObjectRejectsUnknownField(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A thing.
    actions:
      - name: archive
        bogus: nope
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatal("expected an error for an unknown field in an action object, got nil")
	}
}

func TestParseActionRejectsNull(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A thing.
    actions:
      - ~
`
	if _, err := Parse([]byte(src)); err == nil {
		t.Fatal("expected an error for a null action, got nil")
	}
}

func TestEntityNamesNilReceiver(t *testing.T) {
	var m *Model
	if got := m.EntityNames(); got != nil {
		t.Fatalf("EntityNames on nil *Model should return nil, got %v", got)
	}
}

func TestInvertCardinality(t *testing.T) {
	cases := map[string]string{
		"1:1":    "1:1",
		"n:n":    "n:n",
		"1:n":    "n:1",
		"n:1":    "1:n",
		"1:2":    "2:1",
		"0..1:n": "n:0..1",
		"weird":  "weird", // no ":" — returned unchanged
	}
	for in, want := range cases {
		if got := InvertCardinality(in); got != want {
			t.Errorf("InvertCardinality(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseMultiplicity(t *testing.T) {
	cases := []struct {
		in       string
		min, max int
		ok       bool
	}{
		{"1", 1, 1, true},
		{"n", 0, -1, true},
		{"2", 2, 2, true},
		{"0..1", 0, 1, true},
		{"1..n", 1, -1, true},
		{"0..5", 0, 5, true},
		{"5..2", 0, 0, false}, // inverted range
		{"", 0, 0, false},
		{"x", 0, 0, false},
		{"1..x", 0, 0, false},
	}
	for _, c := range cases {
		got, ok := ParseMultiplicity(c.in)
		if ok != c.ok {
			t.Errorf("ParseMultiplicity(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && (got.Min != c.min || got.Max != c.max) {
			t.Errorf("ParseMultiplicity(%q) = {%d,%d}, want {%d,%d}", c.in, got.Min, got.Max, c.min, c.max)
		}
	}
}

func TestParseCardinality(t *testing.T) {
	if _, _, ok := ParseCardinality("1:2"); !ok {
		t.Error("ParseCardinality(1:2) should be ok")
	}
	if _, _, ok := ParseCardinality("nocolon"); ok {
		t.Error("ParseCardinality(nocolon) should not be ok")
	}
	if _, _, ok := ParseCardinality("1:5..2"); ok {
		t.Error("ParseCardinality with an inverted range should not be ok")
	}
}

// TestParseImportForms covers the imports union: a bare path binds the scope its
// filename yields, an object names the scope outright, and a derived scope is
// marked so the linter can tell the two apart.
func TestParseImportForms(t *testing.T) {
	t.Parallel()

	src := `
kind: DomainModel
version: v1
imports:
  - ./payments.modelith.yaml
  - scope: billing
    path: ./legacy/pay-v2.modelith.yaml
entities:
  Project:
    definition: A thing.
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Import{
		{Scope: "payments", Path: "./payments.modelith.yaml", ScopeFromPath: true},
		{Scope: "billing", Path: "./legacy/pay-v2.modelith.yaml"},
	}
	if len(m.Imports) != len(want) {
		t.Fatalf("expected %d imports, got %+v", len(want), m.Imports)
	}
	for i, w := range want {
		if m.Imports[i] != w {
			t.Errorf("import %d: got %+v, want %+v", i, m.Imports[i], w)
		}
	}
}

func TestParseRejectsMalformedImport(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"null entry":      "imports:\n  - ~\n",
		"unknown field":   "imports:\n  - {scope: a, path: b, extra: c}\n",
		"wrong item type": "imports:\n  - [./a.modelith.yaml]\n",
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte("kind: DomainModel\nversion: v1\n" + block)); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestScopeFromPath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"./payments.modelith.yaml":        "payments",
		"../shared/billing.modelith.yaml": "billing",
		"./legacy/pay-v2.yaml":            "pay-v2",
		"./Payments.modelith.yaml":        "Payments",
		"noextension":                     "noextension",
	}
	for in, want := range cases {
		if got := ScopeFromPath(in); got != want {
			t.Errorf("ScopeFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderedPath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"./payments.modelith.yaml": "./payments.modelith.md",
		"../a/b.yml":               "../a/b.md",
		"weird":                    "weird.md",
	}
	for in, want := range cases {
		if got := RenderedPath(in); got != want {
			t.Errorf("RenderedPath(%q) = %q, want %q", in, got, want)
		}
	}
}
