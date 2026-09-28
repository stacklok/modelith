// Package html renders an offline, interactive domain-model viewer.
package html

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

	"github.com/stacklok/modelith/internal/model"
)

//go:embed shell.html
var shell string

//go:embed viewer.css
var css string

//go:embed viewer.js
var js string

type namedEntity struct {
	Name          string               `json:"name"`
	External      bool                 `json:"external"`
	Definition    string               `json:"definition"`
	SubtypeOf     string               `json:"subtypeOf"`
	Derived       bool                 `json:"derived"`
	Derivation    string               `json:"derivation"`
	Attributes    []attribute          `json:"attributes"`
	Relationships []model.Relationship `json:"relationships"`
	Actions       []model.Action       `json:"actions"`
	Invariants    []model.Invariant    `json:"invariants"`
}

type attribute struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    *bool  `json:"required"`
	Description string `json:"description"`
	Derived     bool   `json:"derived"`
	Derivation  string `json:"derivation"`
}

type namedEnum struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Values      []model.EnumValue `json:"values"`
}

type term struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

type view struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Entities    []namedEntity     `json:"entities"`
	Enums       []namedEnum       `json:"enums"`
	Glossary    []term            `json:"glossary"`
	Imports     []model.Import    `json:"imports"`
	Invariants  []model.Invariant `json:"invariants"`
	Scenarios   []model.Scenario  `json:"scenarios"`
}

// Render produces one self-contained HTML document. Model text is serialized
// only into a JSON script with encoding/json's default HTML escaping enabled.
func Render(m *model.Model) string {
	v := view{Title: m.Title, Description: m.Description, Invariants: m.Invariants, Scenarios: m.Scenarios}
	external := map[string]bool{}
	for _, name := range m.EntityNames() {
		e := m.Entities[name]
		n := namedEntity{Name: name, Definition: e.Definition, SubtypeOf: e.SubtypeOf, Derived: e.Derived,
			Derivation: e.Derivation, Relationships: e.Relationships, Actions: e.Actions, Invariants: e.Invariants}
		for _, a := range e.Attributes {
			n.Attributes = append(n.Attributes, attribute{Name: a.Name, Type: a.Type, Description: a.Description,
				Derived: a.Derived, Derivation: a.Derivation})
		}
		if strings.Contains(e.SubtypeOf, ".") {
			external[e.SubtypeOf] = true
		}
		for _, r := range e.Relationships {
			if strings.Contains(r.Entity, ".") {
				external[r.Entity] = true
			}
		}
		v.Entities = append(v.Entities, n)
	}
	for name := range external {
		if _, local := m.Entities[name]; !local {
			v.Entities = append(v.Entities, namedEntity{Name: name, External: true})
		}
	}
	sort.Slice(v.Entities, func(i, j int) bool { return v.Entities[i].Name < v.Entities[j].Name })
	for name, e := range m.Enums {
		v.Enums = append(v.Enums, namedEnum{Name: name, Description: e.Description, Values: e.Values})
	}
	sort.Slice(v.Enums, func(i, j int) bool { return v.Enums[i].Name < v.Enums[j].Name })
	for name, definition := range m.Glossary {
		v.Glossary = append(v.Glossary, term{Name: name, Definition: definition})
	}
	sort.Slice(v.Glossary, func(i, j int) bool { return v.Glossary[i].Name < v.Glossary[j].Name })
	v.Imports = append(v.Imports, m.Imports...)
	sort.Slice(v.Imports, func(i, j int) bool {
		if v.Imports[i].Scope != v.Imports[j].Scope {
			return v.Imports[i].Scope < v.Imports[j].Scope
		}
		return v.Imports[i].Path < v.Imports[j].Path
	})
	payload, err := json.Marshal(v)
	if err != nil {
		// view contains only JSON-compatible concrete fields.
		return ""
	}
	out := strings.Replace(shell, "/* VIEWER_CSS */", css, 1)
	out = strings.Replace(out, "/* VIEWER_JS */", js, 1)
	return strings.Replace(out, "VIEWER_JSON_PAYLOAD", string(payload), 1)
}
