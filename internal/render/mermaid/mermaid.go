// Package mermaid renders a domain model's entities and relationships as a
// Mermaid erDiagram. It emits diagram source only (no fences); callers wrap it
// in a ```mermaid block.
package mermaid

import (
	"fmt"
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

// merge folds a reciprocal declaration into e, choosing the label the single
// drawn line carries. The owning end wins, because its role names the
// relationship the composition is about; with neither end owning, the end whose
// entity sorts first wins, so the choice never depends on declaration order.
// The role the diagram drops is still in that entity's relationship list in the
// Markdown — the diagram is a declaredly lossy view (ADR-0002).
func (e *edge) merge(from, label string, owned bool) {
	switch {
	case owned && !e.owned:
		e.label = label
	case e.owned && !owned:
		// The existing edge is the owning end; its label stands.
	case from < e.from:
		e.label = label
	}
	e.owned = e.owned || owned
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

	// A fold is a claim that two declarations are one relationship seen from two
	// sides. That claim is only safe when each end declares the line at most
	// once: with two declarations on one end, which is the reciprocal of which
	// is undeterminable, and folding either would drop the other's role
	// depending on declaration order. model.EdgeGroups makes that call, and
	// `modelith lint` warns about the same groups (ADR-0008).
	ambiguous := map[string]bool{}
	for _, g := range model.EdgeGroups(m) {
		if g.AmbiguousPairing() {
			ambiguous[g.Key] = true
		}
	}

	var edges []*edge
	byKey := map[string][]*edge{}
	seen := map[string]bool{}
	for _, name := range m.EntityNames() {
		for _, rel := range m.Entities[name].Relationships {
			if rel.Entity == name {
				continue // already rendered inside the entity's block
			}
			// Two declarations from one end that agree on everything draw two
			// indistinguishable lines; the second carries nothing.
			dk := model.DeclarationKey(name, rel)
			if seen[dk] {
				continue
			}
			seen[dk] = true

			label := relationshipLabel(rel)
			owned := rel.Ownership == "owned"
			key := model.EdgeKey(name, rel)

			// A reciprocal folds when the pairing is unambiguous, it comes from
			// the opposite end, and at most one end claims `owned` — mutual
			// `owned` is a contradiction, not a fold, and stays two lines with
			// a lint error against it.
			if !ambiguous[key] {
				folded := false
				for _, e := range byKey[key] {
					if e.from == name || (e.owned && owned) {
						continue
					}
					e.merge(name, label, owned)
					folded = true
					break
				}
				if folded {
					continue
				}
			}
			e := &edge{from: name, to: rel.Entity, card: rel.Cardinality, label: label, owned: owned}
			byKey[key] = append(byKey[key], e)
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
// disambiguate two attributes sharing a name. Declarations that would render
// the same row are emitted once: two indistinguishable rows carry no more
// information than one.
func selfRows(name string, rels []model.Relationship) []string {
	var rows []string
	seen := map[string]bool{}
	for _, rel := range rels {
		if rel.Entity != name {
			continue
		}
		comment := selfComment(rel)
		if seen[comment] {
			continue
		}
		seen[comment] = true
		attr := "self"
		if n := len(rows) + 1; n > 1 {
			attr = "self" + strconv.Itoa(n)
		}
		rows = append(rows, fmt.Sprintf("%s %s %q", name, attr, comment))
	}
	return rows
}

// selfComment is the comment column of a self-relationship row: the declared
// cardinality, the ownership when owned (the line style that carries it
// elsewhere has no line here), then the role.
//
// Both sides of the cardinality are shown. The row stands in for an edge whose
// two end markers encoded both ends, so showing only the target side would drop
// the declaring side — the one piece of information the row exists to preserve.
func selfComment(rel model.Relationship) string {
	out := rel.Cardinality
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
// them for node/attribute syntax; backticks, quotes and backslashes are
// neutralized and newlines collapsed. Dropping the backslash also keeps the %q
// the callers emit with from doubling it into a visible "\\". Entity names
// interpolated elsewhere are constrained to PascalCase by the schema, so they
// need no escaping.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}
