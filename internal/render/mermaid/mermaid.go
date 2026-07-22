// Package mermaid renders a domain model's entities and relationships as a
// Mermaid erDiagram. It emits diagram source only (no fences); callers wrap it
// in a ```mermaid block.
package mermaid

import (
	"fmt"
	"sort"
	"strings"

	"github.com/stacklok/modelith/internal/model"
)

// erMarkers renders a cardinality as Mermaid erDiagram crow's-foot notation,
// using the nearest glyph the notation can express. Mermaid has no numeric
// bound, so an exact or bounded count (e.g. "1:2") renders as one-or-many; the
// precise count lives in the Markdown table and role instead (see ADR-0002).
// An unparseable cardinality falls back to many-to-many, matching the schema's
// pre-validation expectation that it is already a structural error.
func erMarkers(card string) string {
	left, right, ok := model.ParseCardinality(card)
	if !ok {
		return "}o--o{"
	}
	return leftMarker(left) + "--" + rightMarker(right)
}

// minChar is the innermost glyph (nearest the relationship line): "o" for a
// minimum of zero, "|" for one or more.
func minChar(m model.Multiplicity) string {
	if m.Min == 0 {
		return "o"
	}
	return "|"
}

// isMany reports whether the maximum is more than one (or unbounded), which the
// crow's-foot ("{" / "}") represents.
func isMany(m model.Multiplicity) bool {
	return m.Max < 0 || m.Max > 1
}

// leftMarker is the declaring entity's marker: outer (max) glyph then inner
// (min) glyph.
func leftMarker(m model.Multiplicity) string {
	if isMany(m) {
		return "}" + minChar(m)
	}
	return "|" + minChar(m)
}

// rightMarker is the target entity's marker: inner (min) glyph then outer (max)
// glyph.
func rightMarker(m model.Multiplicity) string {
	if isMany(m) {
		return minChar(m) + "{"
	}
	return minChar(m) + "|"
}

// ER renders the model as a Mermaid erDiagram. Attributes are intentionally
// omitted: their freeform conceptual types (e.g. "enum[active, archived]")
// aren't valid erDiagram attribute types. Attributes are shown in the Markdown
// table instead.
func ER(m *model.Model) string {
	var b strings.Builder
	b.WriteString("erDiagram\n")

	// Declare every entity so unconnected ones still appear.
	for _, name := range m.EntityNames() {
		fmt.Fprintf(&b, "    %s {}\n", name)
	}

	seen := map[string]bool{}
	for _, name := range m.EntityNames() {
		for _, rel := range m.Entities[name].Relationships {
			notation := erMarkers(rel.Cardinality)
			label := relationshipLabel(rel)

			// Dedupe edges declared from both sides of the same pair. The key
			// includes the cardinality normalized to the sorted-pair
			// orientation, so a relationship declared from both sides with
			// inverse cardinalities (A "1:n" B, B "n:1" A) collapses to one
			// edge — while genuinely distinct edges (GO-3) or contradictory
			// reciprocal declarations (GO-1) keep distinct keys and both
			// render, surfacing the conflict instead of silently dropping one.
			// `modelith lint` reports the contradiction as an error.
			pair := []string{name, rel.Entity}
			sort.Strings(pair)
			card := rel.Cardinality
			if name != pair[0] {
				card = model.InvertCardinality(card)
			}
			key := pair[0] + "\x00" + pair[1] + "\x00" + card + "\x00" + label
			if seen[key] {
				continue
			}
			seen[key] = true

			fmt.Fprintf(&b, "    %s %s %s : %q\n", name, notation, rel.Entity, label)
		}
	}
	return b.String()
}

func relationshipLabel(rel model.Relationship) string {
	switch {
	case rel.Role != "":
		return sanitize(rel.Role)
	case rel.Ownership != "":
		return rel.Ownership
	default:
		return rel.Cardinality
	}
}

// sanitize strips or replaces characters that would break a quoted Mermaid
// label. Square brackets are replaced with parentheses because Mermaid uses
// them for node/attribute syntax; backticks and quotes are neutralized and
// newlines collapsed. Entity names interpolated elsewhere are constrained to
// PascalCase by the schema, so they need no escaping.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
