package model

import (
	"sort"
	"strings"
)

// This file holds the one definition of "which relationship line does this
// declaration describe". The renderer uses it to decide what to fold into one
// line; the linter uses it to warn when that decision cannot be made. Keeping
// both on the same code is deliberate: a diagram whose fold rule and diagnostic
// disagree is worse than either alone.

// NormalizeRole reduces a role to its text: backticks are markup, and
// surrounding or repeated whitespace is not content. Two declarations whose
// roles normalize equal are the same role.
func NormalizeRole(role string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(role, "`", "")), " ")
}

// EdgeKey identifies the relationship line a declaration would draw: the
// unordered entity pair plus the cardinality canonicalized to that pair's
// orientation. Two declarations share a key exactly when they could be the same
// line — including one relationship declared from both ends with inverse
// cardinalities ("1:n" one way, "n:1" the other). A self-referential
// declaration has no key; those are drawn inside the entity's own block.
func EdgeKey(from string, rel Relationship) string {
	pair := []string{from, rel.Entity}
	sort.Strings(pair)
	card := rel.Cardinality
	if from != pair[0] {
		card = InvertCardinality(card)
	}
	return pair[0] + "\x00" + pair[1] + "\x00" + CanonicalCardinality(card)
}

// DeclarationKey identifies a declaration precisely enough that two sharing it
// are indistinguishable wherever the model is drawn: the line, the end that
// declared it, whether that end claims ownership, and the role.
func DeclarationKey(from string, rel Relationship) string {
	owned := "0"
	if rel.Ownership == "owned" {
		owned = "1"
	}
	return EdgeKey(from, rel) + "\x00" + from + "\x00" + owned + "\x00" + NormalizeRole(rel.Role)
}

// Declaration is one relationship declaration together with where it was made,
// so a caller can point a diagnostic at it.
type Declaration struct {
	From  string // the entity whose relationships list holds it
	Index int    // its position in that list
	Rel   Relationship
}

// EdgeGroup is every declaration that could draw one relationship line. First
// holds the declarations made by the alphabetically earlier entity of the pair,
// Second those made by the later one; either may be empty.
type EdgeGroup struct {
	Key             string
	First, Second   []Declaration
	FirstN, SecondN string // the two entity names, First's and Second's
}

// AmbiguousPairing reports whether the group's declarations cannot be paired up
// across the two ends. Both ends declare the line, but at least one of them
// declares it more than once, so which declaration is the reciprocal of which
// is undeterminable — the format has no way to say. A caller must not guess:
// picking one pairing silently drops the role of whichever declaration it
// overwrote, and which one that is would depend on declaration order.
func (g *EdgeGroup) AmbiguousPairing() bool {
	if len(g.First) == 0 || len(g.Second) == 0 {
		return false
	}
	return len(g.First) > 1 || len(g.Second) > 1
}

// EdgeGroups groups a model's non-self relationship declarations by the line
// each would draw, dropping any declaration indistinguishable from an earlier
// one made by the same entity (an exact duplicate carries no information the
// first does not). Groups come back in sorted-key order, and the declarations
// within a group in declaration order — entity name, then position — so a
// caller's output is deterministic.
func EdgeGroups(m *Model) []*EdgeGroup {
	byKey := map[string]*EdgeGroup{}
	seen := map[string]bool{}
	for _, name := range m.EntityNames() {
		for i, rel := range m.Entities[name].Relationships {
			if rel.Entity == name {
				continue // drawn inside the entity's own block, never as a line
			}
			dk := DeclarationKey(name, rel)
			if seen[dk] {
				continue
			}
			seen[dk] = true

			key := EdgeKey(name, rel)
			g, ok := byKey[key]
			if !ok {
				pair := []string{name, rel.Entity}
				sort.Strings(pair)
				g = &EdgeGroup{Key: key, FirstN: pair[0], SecondN: pair[1]}
				byKey[key] = g
			}
			d := Declaration{From: name, Index: i, Rel: rel}
			if name == g.FirstN {
				g.First = append(g.First, d)
			} else {
				g.Second = append(g.Second, d)
			}
		}
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]*EdgeGroup, 0, len(byKey))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}
