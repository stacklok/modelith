package html

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/model"
)

func example(t *testing.T) *model.Model {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "example.modelith.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func payload(t *testing.T, page string) view {
	t.Helper()
	const start = `<script id="model-data" type="application/json">`
	_, after, ok := strings.Cut(page, start)
	if !ok {
		t.Fatal("missing JSON script")
	}
	content, _, ok := strings.Cut(after, "</script>")
	if !ok {
		t.Fatal("unclosed JSON script")
	}
	var v view
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRender_Golden(t *testing.T) {
	t.Parallel()
	page := Render(example(t))
	golden, err := os.ReadFile(filepath.Join("testdata", "example.html"))
	if err != nil {
		t.Fatal(err)
	}
	if page != string(golden) {
		t.Fatal("HTML differs from testdata/example.html; regenerate deliberately and review")
	}
}

func TestRender_DeterministicMaps(t *testing.T) {
	t.Parallel()
	m := example(t)
	m.Enums["Other"] = model.Enum{Values: []model.EnumValue{{Name: "another"}}}
	m.Imports = []model.Import{{Scope: "z", Path: "z.yaml"}, {Scope: "a", Path: "a.yaml"}}
	first := Render(m)
	reordered := &model.Model{Title: m.Title, Description: m.Description, Enums: map[string]model.Enum{}, Glossary: map[string]string{}, Entities: map[string]model.Entity{}, Imports: []model.Import{m.Imports[1], m.Imports[0]}, Invariants: m.Invariants, Scenarios: m.Scenarios}
	for _, name := range slices.Backward(m.EntityNames()) {
		reordered.Entities[name] = m.Entities[name]
	}
	enums := make([]string, 0, len(m.Enums))
	for name := range m.Enums {
		enums = append(enums, name)
	}
	slices.Sort(enums)
	for _, name := range slices.Backward(enums) {
		reordered.Enums[name] = m.Enums[name]
	}
	terms := make([]string, 0, len(m.Glossary))
	for name := range m.Glossary {
		terms = append(terms, name)
	}
	slices.Sort(terms)
	for _, name := range slices.Backward(terms) {
		reordered.Glossary[name] = m.Glossary[name]
	}
	if first != Render(m) || first != Render(reordered) {
		t.Fatal("render is not deterministic")
	}
}

func TestRender_EscapesAndPreservesModelText(t *testing.T) {
	t.Parallel()
	hostile := "<script></script>&\"'`"
	m := &model.Model{Title: hostile, Description: hostile, Entities: map[string]model.Entity{
		hostile: {Definition: hostile, Relationships: []model.Relationship{{Entity: "remote.Target", Cardinality: "1:0..1", Role: hostile, Note: hostile}}},
	}}
	page := Render(m)
	if strings.Count(page, "</script>") != 2 || strings.Contains(page, hostile) || !strings.Contains(page, `\u003cscript\u003e`) {
		t.Fatal("unescaped model text or script breakout")
	}
	v := payload(t, page)
	if v.Title != hostile || v.Description != hostile || v.Entities[0].Definition != hostile || v.Entities[0].Relationships[0].Role != hostile || v.Entities[0].Relationships[0].Note != hostile {
		t.Fatalf("model text did not roundtrip: %+v", v)
	}
}

func TestRender_GraphPayload(t *testing.T) {
	t.Parallel()
	m := &model.Model{Entities: map[string]model.Entity{
		"Child": {SubtypeOf: "remote.Parent", Derived: true, Derivation: "calculated", Attributes: []model.Attribute{{Name: "total", Type: "integer", Derived: true, Derivation: "sum"}}, Relationships: []model.Relationship{{Entity: "Child", Cardinality: "1:0..1", Role: "previous", Symmetric: true, Ownership: "owned", Note: "self"}, {Entity: "remote.Target", Cardinality: "2:n", Role: "target"}}},
	}}
	v := payload(t, Render(m))
	if len(v.Entities) != 3 || v.Entities[0].Name != "Child" || v.Entities[0].Attributes[0].Required != nil || !v.Entities[1].External || v.Entities[1].Name != "remote.Parent" || !v.Entities[2].External || v.Entities[2].Name != "remote.Target" {
		t.Fatalf("graph normalization: %+v", v.Entities)
	}
	if !reflect.DeepEqual(v.Entities[0].Relationships, m.Entities["Child"].Relationships) || v.Entities[0].SubtypeOf != "remote.Parent" || !v.Entities[0].Derived || v.Entities[0].Attributes[0].Derivation != "sum" {
		t.Fatal("lost relationship, subtype or derived data")
	}
}

func TestADR_0020_SelfContainedHTML(t *testing.T) {
	t.Parallel()
	page := strings.ReplaceAll(Render(example(t)), "http://www.w3.org/2000/svg", "")
	for _, forbidden := range []string{"innerHTML", "fetch(", "XMLHttpRequest", "<script src=", "<link ", "@import", "http://", "https://"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("viewer contains external resource or unsafe DOM API %q", forbidden)
		}
	}
	if strings.Count(page, "<script") != 2 || strings.Count(page, "</script>") != 2 || !strings.Contains(page, "textContent") || !strings.Contains(page, "createElementNS") {
		t.Fatal("viewer must contain exactly the inline JSON and JS scripts and use text DOM APIs")
	}
}
