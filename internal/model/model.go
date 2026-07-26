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
	"path"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

// Model is the top-level domain model document.
type Model struct {
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Shared marks a model other models import. The references that justify its
	// vocabulary then live in files it cannot see, so the completeness checks
	// that flag a definition nothing here uses do not apply to it. It is the
	// counterpart of Imports: one model declares what it reaches for, the other
	// declares that it is reached for.
	Shared bool `json:"shared,omitempty"`
	// Imports are the other model files this one references, each bound to the
	// scope written at its reference sites. Resolution does not recurse: an
	// imported model's own imports are not reachable from here (ADR-0010,
	// ADR-0012).
	Imports   []Import          `json:"imports,omitempty"`
	Glossary  map[string]string `json:"glossary,omitempty"`
	Enums     map[string]Enum   `json:"enums,omitempty"`
	Entities  map[string]Entity `json:"entities,omitempty"`
	Scenarios []Scenario        `json:"scenarios,omitempty"`
	// Invariants are model-level rules that span several entities and have no
	// single owner. They share the per-entity invariant shape, and their ids
	// share one namespace with entity invariants (unique across the model).
	Invariants []Invariant `json:"invariants,omitempty"`
}

// ScopeSlug is the one definition of a valid import scope, as an unanchored
// regexp source so callers can embed it in a larger pattern ("scope.Name").
// The schema's `scope` pattern is this anchored, and
// TestInvariant_ScopeSlugMatchesSchema guards the two against drift.
const ScopeSlug = `[a-z][a-z0-9-]*`

// Import is another model file this one references, bound to the scope written
// before an imported item's name ("payments.PaymentMethod"). The binding is the
// importer's: the imported file says nothing about how it is named here, so two
// models may bind the same file to different scopes (ADR-0012).
type Import struct {
	Scope string `json:"scope"`
	Path  string `json:"path"`
	// ScopeFromPath records that Scope came from Path's basename rather than
	// being written out, so the linter can point at the explicit form when a
	// filename yields an unusable slug. The schema validates an explicit scope
	// itself; a derived one it never sees.
	ScopeFromPath bool `json:"-"`
}

// UnmarshalJSON lets an import be written as a bare path ("./payments.modelith.yaml",
// whose basename gives the scope) or as an object naming the scope explicitly
// ({scope: billing, path: ./legacy/pay-v2.modelith.yaml}).
func (i *Import) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("import must be a path string or an object, not null")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*i = Import{Scope: ScopeFromPath(s), Path: s, ScopeFromPath: true}
		return nil
	}
	type rawImport Import
	var r rawImport
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return err
	}
	*i = Import(r)
	return nil
}

// ScopeFromPath derives the scope a bare import path binds: the basename with
// ".modelith.yaml" or ".modelith.yml" — or failing that, the final extension —
// stripped. Both spellings are stripped whole, because trimming ".yml" alone
// leaves "payments.modelith", which is no slug at all. The result is not
// guaranteed to be a valid slug; the linter reports one that isn't, since only
// an explicitly written scope passes through the schema.
func ScopeFromPath(p string) string {
	base := path.Base(p)
	for _, suffix := range []string{".modelith.yaml", ".modelith.yml"} {
		if trimmed, ok := strings.CutSuffix(base, suffix); ok {
			return trimmed
		}
	}
	return strings.TrimSuffix(base, path.Ext(base))
}

// RenderedPath returns where `modelith render` writes a model file's Markdown:
// the same path with a .yaml/.yml extension replaced by .md.
func RenderedPath(p string) string {
	ext := path.Ext(p)
	if ext == ".yaml" || ext == ".yml" {
		return strings.TrimSuffix(p, ext) + ".md"
	}
	return p + ".md"
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
	Definition string `json:"definition"`
	// SubtypeOf names the entity this one is a kind of (an is-a link). The
	// parent's invariants are understood to cover this entity too.
	SubtypeOf     string         `json:"subtypeOf,omitempty"`
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
	// Symmetric marks a relationship as carrying no inherent order, so (a, b)
	// is the same as (b, a) — an unordered pair, peering, or adjacency.
	Symmetric bool   `json:"symmetric,omitempty"`
	Role      string `json:"role,omitempty"`
	Ownership string `json:"ownership,omitempty"`
	Note      string `json:"note,omitempty"`
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
// relationship by swapping the two sides: "1:n" becomes "n:1", "1:2" becomes
// "2:1", while "1:1" and "n:n" are unchanged. A value with no ":" is returned
// unchanged. This lets the renderer dedupe a relationship declared from both
// sides and lets the linter check that reciprocal declarations agree.
func InvertCardinality(c string) string {
	left, right, ok := strings.Cut(c, ":")
	if !ok {
		return c
	}
	return right + ":" + left
}

// Multiplicity is one side of a relationship cardinality parsed into a numeric
// range. Max is -1 when unbounded ("many").
type Multiplicity struct {
	Min int
	Max int // -1 == unbounded
}

// ParseMultiplicity parses one side of a cardinality string: "1", "n", an exact
// count like "2", or a range like "0..1", "1..n", "0..5". ok is false for a
// malformed side or an inverted range (min greater than max).
func ParseMultiplicity(s string) (Multiplicity, bool) {
	if s == "n" {
		return Multiplicity{Min: 0, Max: -1}, true
	}
	lo, hi, isRange := strings.Cut(s, "..")
	if !isRange {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return Multiplicity{}, false
		}
		return Multiplicity{Min: n, Max: n}, true
	}
	min, err := strconv.Atoi(lo)
	if err != nil || min < 0 {
		return Multiplicity{}, false
	}
	if hi == "n" {
		return Multiplicity{Min: min, Max: -1}, true
	}
	max, err := strconv.Atoi(hi)
	if err != nil || max < min {
		return Multiplicity{}, false
	}
	return Multiplicity{Min: min, Max: max}, true
}

// ParseCardinality splits an "left:right" cardinality and parses both sides. ok
// is false if the string has no ":" or either side is malformed.
func ParseCardinality(c string) (left, right Multiplicity, ok bool) {
	a, b, found := strings.Cut(c, ":")
	if !found {
		return Multiplicity{}, Multiplicity{}, false
	}
	l, lok := ParseMultiplicity(a)
	r, rok := ParseMultiplicity(b)
	return l, r, lok && rok
}

// canonical is a normal-form string for one multiplicity, so semantically equal
// sides written differently ("n" and "0..n") compare equal.
func (m Multiplicity) canonical() string {
	return strconv.Itoa(m.Min) + ".." + strconv.Itoa(m.Max)
}

// CanonicalCardinality returns a normal form in which semantically equal
// cardinalities written differently ("1:n" and "1:0..n") are the same string.
// An unparseable value is returned unchanged so it still compares by its raw
// text.
func CanonicalCardinality(c string) string {
	l, r, ok := ParseCardinality(c)
	if !ok {
		return c
	}
	return l.canonical() + ":" + r.canonical()
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
