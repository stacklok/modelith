// Package mermaid renders a domain model's entities and relationships as a
// Mermaid erDiagram. It emits diagram source only (no fences); callers wrap it
// in a ```mermaid block.
package mermaid

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/stacklok/modelith/internal/model"
)

// erMarkers renders a cardinality as Mermaid erDiagram crow's-foot notation,
// using the nearest glyph the notation can express. Mermaid has no numeric
// bound, so an exact or bounded count (e.g. "1:2") renders as one-or-many; the
// precise count lives in the Markdown table and role instead (see ADR-0002).
// An unparseable cardinality falls back to many-to-many, matching the schema's
// pre-validation expectation that it is already a structural error.
//
// owned selects the connector: an identifying line ("--") for composition, a
// non-identifying one ("..") otherwise, so ownership costs no label space
// (ADR-0008).
func erMarkers(card string, owned bool) string {
	line := connector(owned)
	left, right, ok := model.ParseCardinality(card)
	if !ok {
		return "}o" + line + "o{"
	}
	return leftMarker(left) + line + rightMarker(right)
}

// connector is the Mermaid relationship line: identifying (solid) when the
// related entity is owned, non-identifying (dashed) otherwise. The schema's
// default for an omitted ownership is "referenced", so omitted draws dashed.
func connector(owned bool) string {
	if owned {
		return "--"
	}
	return ".."
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

// edge is one rendered relationship line, accumulated before emission so that
// declarations of the same relationship from both ends fold into it.
type edge struct {
	from, to string
	card     string // as declared by `from`, so the markers read left to right
	label    string
	owned    bool
}

// ER renders the model as a Mermaid erDiagram. Ordinary attributes are
// intentionally omitted: their freeform conceptual types (e.g.
// "enum[active, archived]") aren't valid erDiagram attribute types, so they are
// shown in the Markdown table instead. The one thing inside an entity block is
// its self-referential relationships (see selfRows).
func ER(m *model.Model) string {
	var b strings.Builder
	b.WriteString("erDiagram\n")

	// Declare every entity so unconnected ones still appear.
	for _, name := range m.EntityNames() {
		rows := selfRows(name, m.Entities[name].Relationships)
		if len(rows) == 0 {
			fmt.Fprintf(&b, "    %s {}\n", name)
			continue
		}
		fmt.Fprintf(&b, "    %s {\n", name)
		for _, row := range rows {
			fmt.Fprintf(&b, "        %s\n", row)
		}
		b.WriteString("    }\n")
	}

	var edges []*edge
	byKey := map[string]*edge{}
	for _, name := range m.EntityNames() {
		for _, rel := range m.Entities[name].Relationships {
			if rel.Entity == name {
				continue // already rendered inside the entity's block
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
			// Canonicalize so a relationship declared from both sides with
			// semantically equal but differently written cardinalities
			// ("1:n" and "0..n:1") dedupes to one edge.
			card = model.CanonicalCardinality(card)
			key := pair[0] + "\x00" + pair[1] + "\x00" + card + "\x00" + label
			if e, ok := byKey[key]; ok {
				// Ownership belongs to the relationship, not to the end that
				// declared it: a parent declaring `owned` and the child
				// declaring `referenced` are one identifying relationship seen
				// from two sides, so the folded edge stays solid (ADR-0008).
				e.owned = e.owned || rel.Ownership == "owned"
				continue
			}
			e := &edge{from: name, to: rel.Entity, card: rel.Cardinality, label: label, owned: rel.Ownership == "owned"}
			byKey[key] = e
			edges = append(edges, e)
		}
	}

	for _, e := range edges {
		fmt.Fprintf(&b, "    %s %s %s : %q\n", e.from, erMarkers(e.card, e.owned), e.to, e.label)
	}
	return b.String()
}

// selfRows renders an entity's self-referential relationships as rows inside
// its own block. Mermaid's dagre ER layout has no self-loop handling, so an
// edge from an entity to itself draws a runaway arc that swamps the canvas
// (issue #26); the row carries the same information without a line (ADR-0008).
// Row names are self, self2, self3… — distinct, since Mermaid does not
// disambiguate two attributes sharing a name.
func selfRows(name string, rels []model.Relationship) []string {
	var rows []string
	for _, rel := range rels {
		if rel.Entity != name {
			continue
		}
		attr := "self"
		if n := len(rows) + 1; n > 1 {
			attr = "self" + strconv.Itoa(n)
		}
		rows = append(rows, fmt.Sprintf("%s %s %q", name, attr, selfComment(rel)))
	}
	return rows
}

// selfComment is the comment column of a self-relationship row: the target-side
// cardinality, the ownership when owned (the line style that carries it
// elsewhere has no line here), then the role.
func selfComment(rel model.Relationship) string {
	target := rel.Cardinality
	if _, right, ok := strings.Cut(rel.Cardinality, ":"); ok {
		target = right
	}
	out := target
	if rel.Ownership == "owned" {
		out += " owned"
	}
	if rel.Role != "" {
		out += " — " + rel.Role
	}
	return sanitize(out)
}

// relationshipLabel is the quoted text on a relationship line: the role, or
// nothing. Ownership rides on the line style instead, and the precise
// cardinality lives in the Markdown table (ADR-0002, ADR-0008).
func relationshipLabel(rel model.Relationship) string {
	return sanitize(rel.Role)
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
