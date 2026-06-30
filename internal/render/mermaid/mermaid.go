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

// cardinalityNotation maps our "a:b" cardinality strings to Mermaid erDiagram
// crow's-foot notation. The left side is the entity declaring the
// relationship; the right side is the target.
var cardinalityNotation = map[string]string{
	"1:1": "||--||",
	"1:n": "||--o{",
	"n:1": "}o--||",
	"n:n": "}o--o{",
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
			notation, ok := cardinalityNotation[rel.Cardinality]
			if !ok {
				notation = "}o--o{"
			}
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
