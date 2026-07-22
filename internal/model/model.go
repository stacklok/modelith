// Package model defines the in-memory representation of a domain model and
// parses it from YAML. Parsing here is intentionally permissive about
// structure — structural validation against the JSON Schema lives in the lint
// package. This package gives the renderer and semantic checks typed access to
// the model.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"sigs.k8s.io/yaml"
)

// Model is the top-level domain model document.
type Model struct {
	Kind        string            `json:"kind"`
	Version     string            `json:"version"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Glossary    map[string]string `json:"glossary,omitempty"`
	Enums       map[string]Enum   `json:"enums,omitempty"`
	Entities    map[string]Entity `json:"entities,omitempty"`
	Scenarios   []Scenario        `json:"scenarios,omitempty"`
	// Invariants are model-level rules that span several entities and have no
	// single owner. They share the per-entity invariant shape, and their ids
	// share one namespace with entity invariants (unique across the model).
	Invariants []Invariant `json:"invariants,omitempty"`
}

// Enum is a named, first-class set of allowed values for an attribute. Defining
// it once (rather than inline in a "type" string) makes the values
// referenceable and checkable.
type Enum struct {
	Description string      `json:"description,omitempty"`
	Values      []EnumValue `json:"values"`
}

// EnumValue is one allowed value of an Enum, optionally with a definition so its
// meaning is part of the ubiquitous language rather than guessed at.
type EnumValue struct {
	Name       string `json:"name"`
	Definition string `json:"definition,omitempty"`
}

// Entity is a named concept in the domain.
type Entity struct {
	Definition    string         `json:"definition"`
	Relationships []Relationship `json:"relationships,omitempty"`
	Attributes    []Attribute    `json:"attributes,omitempty"`
	Actions       []Action       `json:"actions,omitempty"`
	Invariants    []Invariant    `json:"invariants,omitempty"`
	// Derived marks an entity as wholly computed from other state rather than
	// persisted — never stored, recomputed on demand. Unlike a derived
	// attribute, Derivation is optional even when Derived is true: the
	// Definition often already explains how.
	Derived    bool   `json:"derived,omitempty"`
	Derivation string `json:"derivation,omitempty"`
}

// Relationship describes how an entity relates to another.
type Relationship struct {
	Entity      string `json:"entity"`
	Cardinality string `json:"cardinality"`
	Role        string `json:"role,omitempty"`
	Ownership   string `json:"ownership,omitempty"`
	Note        string `json:"note,omitempty"`
}

// Attribute is a key property of an entity.
type Attribute struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	// Derived marks an attribute as computed from other state rather than
	// stored, so newcomers don't model it as a persisted column. When true,
	// Derivation explains how it is computed.
	Derived    bool   `json:"derived,omitempty"`
	Derivation string `json:"derivation,omitempty"`
}

// Action is something that can be done to or by an entity. It accepts either a
// bare string (just the action name) or a structured object linking the action
// to the actor that performs it and the invariants it must preserve.
type Action struct {
	Name        string   `json:"name"`
	Actor       string   `json:"actor,omitempty"`
	Preserves   []string `json:"preserves,omitempty"`
	Description string   `json:"description,omitempty"`
}

// UnmarshalJSON lets an action be written as a bare string ("create") or as a
// structured object ({name: archive, actor: Owner, preserves: [...]}).
func (a *Action) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("action must be a string or an object, not null")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		a.Name = s
		return nil
	}
	type rawAction Action
	var r rawAction
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return err
	}
	*a = Action(r)
	return nil
}

// Invariant is a rule that must always hold for an entity. It carries a stable
// id so scenarios and actions can reference it without depending on the exact
// wording of the statement.
type Invariant struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

// Scenario is a short narrative that exercises the model.
type Scenario struct {
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Actors            []string `json:"actors,omitempty"`
	Steps             []string `json:"steps,omitempty"`
	InvariantsTouched []string `json:"invariants_touched,omitempty"` // invariant ids
}

// Parse decodes a domain model from YAML bytes. It does not validate against
// the schema; use the lint package for that.
func Parse(data []byte) (*Model, error) {
	var m Model
	if err := yaml.UnmarshalStrict(data, &m); err != nil {
		return nil, fmt.Errorf("parsing domain model: %w", err)
	}
	return &m, nil
}

// InvertCardinality returns the cardinality as seen from the other side of the
// relationship: "1:n" becomes "n:1" and vice versa, while "1:1" and "n:n" are
// symmetric. Unrecognized values are returned unchanged. This lets the renderer
// dedupe a relationship declared from both sides and lets the linter check that
// reciprocal declarations agree.
func InvertCardinality(c string) string {
	switch c {
	case "1:n":
		return "n:1"
	case "n:1":
		return "1:n"
	default:
		return c
	}
}

// EntityNames returns the entity keys in a stable (alphabetical) order so that
// rendering is deterministic regardless of map iteration order.
func (m *Model) EntityNames() []string {
	if m == nil {
		return nil
	}
	names := make([]string, 0, len(m.Entities))
	for name := range m.Entities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
