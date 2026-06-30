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
