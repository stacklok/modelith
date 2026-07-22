// Package lint validates a domain model in three layers:
//
//   - Structural: the file conforms to the JSON Schema (types, required
//     fields, enums). These are hard errors.
//   - Semantic: cross-references hold — relationship targets and backticked
//     entity names resolve to defined entities, and scenario invariants match
//     declared ones. Broken references are errors; unresolved freeform terms
//     are advisory warnings.
//   - Completeness: advisory gaps — entities with no invariants, entities no
//     scenario exercises. These surface gaps without demanding perfection.
package lint

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"sigs.k8s.io/yaml"

	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/schema"
)

// Severity classifies how serious a finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Category groups findings by the layer that produced them.
type Category string

const (
	CategoryStructural   Category = "structural"
	CategorySemantic     Category = "semantic"
	CategoryCompleteness Category = "completeness"
)

// Finding is a single lint result.
type Finding struct {
	Severity Severity `json:"severity"`
	Category Category `json:"category"`
	Path     string   `json:"path"`
	Message  string   `json:"message"`
}

// Result is the collected output of a lint run.
type Result struct {
	Findings []Finding `json:"findings"`
}

// HasBlocking reports whether the result should fail a build. Errors always
// block; completeness findings block only when completenessAsError is set.
func (r *Result) HasBlocking(completenessAsError bool) bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
		if completenessAsError && f.Category == CategoryCompleteness {
			return true
		}
	}
	return false
}

var (
	backtickRE   = regexp.MustCompile("`([^`]+)`")
	pascalCaseRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	printer      = message.NewPrinter(language.English)
)

// Run validates the given YAML bytes and returns all findings.
func Run(data []byte) (*Result, error) {
	res := &Result{}

	// Layer 1: structural validation against the JSON Schema.
	structuralOK := runStructural(data, res)

	// If it does not even parse into our typed model, stop — semantic and
	// completeness checks need a model to work with. The structural layer has
	// already reported why.
	m, err := model.Parse(data)
	if err != nil {
		if structuralOK {
			// Schema passed but typed parsing failed; surface it so we never
			// silently skip the later layers.
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategoryStructural,
				Path:     "",
				Message:  err.Error(),
			})
		}
		sortFindings(res)
		return res, nil
	}

	runSemantic(m, res)
	runRelationshipShape(m, res)
	runReciprocity(m, res)
	runCompleteness(m, res)

	sortFindings(res)
	return res, nil
}

// Structural runs only the JSON Schema (structural) layer and returns its
// findings — empty if the document is structurally valid. `modelith render` uses it
// to fail with a friendly schema error instead of the raw strict-YAML parse
// error, without running the semantic/completeness layers (which shouldn't
// block rendering).
func Structural(data []byte) []Finding {
	res := &Result{}
	runStructural(data, res)
	sortFindings(res)
	return res.Findings
}

// runStructural validates against the JSON Schema. Returns true if the document
// is structurally valid.
func runStructural(data []byte, res *Result) bool {
	jsonBytes, err := yaml.YAMLToJSON(data)
	if err != nil {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  fmt.Sprintf("not valid YAML: %v", err),
		})
		return false
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  fmt.Sprintf("could not decode document: %v", err),
		})
		return false
	}

	// Dispatch on the declared format version. modelith — not the schema — is
	// the source of truth for which versions this build understands: if the file
	// targets a version we don't have, say so plainly (so a newer file gets
	// "upgrade modelith", not a cryptic schema mismatch) and validate against
	// that version's schema. A missing/empty version falls through to the schema,
	// which requires it and reports the absence in the usual way.
	version := schema.Current
	if obj, ok := inst.(map[string]any); ok {
		if v, ok := obj["version"].(string); ok && v != "" {
			version = v
			if !schema.Supported(v) {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategoryStructural,
					Path:     "/version",
					Message: fmt.Sprintf("unsupported schema version %q; this modelith supports: %s "+
						"(upgrade modelith, or set a supported version)", v, strings.Join(schema.SupportedVersions(), ", ")),
				})
				return false
			}
		}
	}

	sch, err := schema.CompileVersion(version)
	if err != nil {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  fmt.Sprintf("internal: %v", err),
		})
		return false
	}

	if err := sch.Validate(inst); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			before := len(res.Findings)
			collectLeaves(ve, res)
			return len(res.Findings) == before
		}
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  err.Error(),
		})
		return false
	}
	return true
}

func collectLeaves(e *jsonschema.ValidationError, res *Result) {
	if len(e.Causes) == 0 {
		ptr := "/" + strings.Join(e.InstanceLocation, "/")
		if ptr == "/" {
			ptr = ""
		}
		msg := e.Error()
		if e.ErrorKind != nil {
			msg = e.ErrorKind.LocalizedString(printer)
		}
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Path:     ptr,
			Message:  msg,
		})
		return
	}
	for _, c := range e.Causes {
		collectLeaves(c, res)
	}
}

func runSemantic(m *model.Model, res *Result) {
	entitySet := map[string]bool{}
	for name := range m.Entities {
		entitySet[name] = true
	}
	// allowed maps a backtick token (entity name or its naive plural) to the
	// canonical entity it refers to.
	allowed := map[string]string{}
	for name := range m.Entities {
		allowed[name] = name
		allowed[plural(name)] = name
	}

	// Glossary terms are defined non-entity vocabulary (roles, states, nouns).
	glossary := map[string]bool{}
	for term := range m.Glossary {
		glossary[term] = true
	}

	// Enums are referenceable types (an attribute names one in its `type`).
	enums := map[string]bool{}
	for name := range m.Enums {
		enums[name] = true
	}

	// Invariant ids, collected across both scopes — per-entity and model-level.
	// Ids share one namespace and must be unique so a reference
	// (invariants_touched, action.preserves) resolves unambiguously, regardless
	// of which scope declared the invariant.
	invariantID := map[string]bool{}
	dupInvariant := func(id, path string) {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategorySemantic,
			Path:     path,
			Message:  fmt.Sprintf("duplicate invariant id %q — ids must be unique so references resolve unambiguously", id),
		})
	}
	for _, name := range m.EntityNames() {
		for i, inv := range m.Entities[name].Invariants {
			if inv.ID == "" {
				continue // schema requires it; the structural layer reports absence
			}
			if invariantID[inv.ID] {
				dupInvariant(inv.ID, fmt.Sprintf("/entities/%s/invariants/%d/id", name, i))
				continue
			}
			invariantID[inv.ID] = true
		}
	}
	for i, inv := range m.Invariants {
		if inv.ID == "" {
			continue // schema requires it; the structural layer reports absence
		}
		if invariantID[inv.ID] {
			dupInvariant(inv.ID, fmt.Sprintf("/invariants/%d/id", i))
			continue
		}
		invariantID[inv.ID] = true
	}

	// Relationship targets must reference defined entities.
	for _, name := range m.EntityNames() {
		ent := m.Entities[name]
		for i, rel := range ent.Relationships {
			if !entitySet[rel.Entity] {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/relationships/%d/entity", name, i),
					Message:  fmt.Sprintf("relationship targets undefined entity %q", rel.Entity),
				})
			}
			// A role names a non-entity vocabulary term; it should resolve to an
			// entity or a glossary term (the DDD-1 payoff — undefined roles).
			for _, base := range entityRefs(rel.Role) {
				if allowed[base] != "" || glossary[base] {
					continue
				}
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityWarning,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/relationships/%d/role", name, i),
					Message:  fmt.Sprintf("role term %q is not a defined entity or glossary term — define it in the glossary", base),
				})
			}
		}
	}

	// Known non-entity vocabulary for backtick resolution: glossary terms,
	// declared scenario actors (which may be ad-hoc participants like
	// `TargetUser` and are intentionally not required to be glossary terms), and
	// role terms (so they don't double-warn in freeform text).
	knownNonEntity := map[string]bool{}
	for term := range glossary {
		knownNonEntity[term] = true
	}
	for _, sc := range m.Scenarios {
		for _, actor := range sc.Actors {
			knownNonEntity[strings.TrimSpace(actor)] = true
		}
	}
	for _, ent := range m.Entities {
		for _, rel := range ent.Relationships {
			for _, base := range entityRefs(rel.Role) {
				knownNonEntity[base] = true
			}
		}
	}

	// Backticked entity-looking terms in freeform text should resolve to a
	// defined entity, glossary term, declared role, or actor.
	checkRefs := func(path, text string) {
		for _, base := range entityRefs(text) {
			if allowed[base] != "" || knownNonEntity[base] {
				continue
			}
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarning,
				Category: CategorySemantic,
				Path:     path,
				Message:  fmt.Sprintf("backticked term %q is not a defined entity, glossary term, role, or actor", base),
			})
		}
	}
	for _, term := range sortedMapKeys(m.Glossary) {
		checkRefs(fmt.Sprintf("/glossary/%s", term), m.Glossary[term])
	}
	for _, ename := range sortedMapKeys(m.Enums) {
		en := m.Enums[ename]
		checkRefs(fmt.Sprintf("/enums/%s/description", ename), en.Description)
		for i, v := range en.Values {
			checkRefs(fmt.Sprintf("/enums/%s/values/%d/definition", ename, i), v.Definition)
		}
	}
	for _, name := range m.EntityNames() {
		ent := m.Entities[name]
		checkRefs(fmt.Sprintf("/entities/%s/definition", name), ent.Definition)
		for i, rel := range ent.Relationships {
			checkRefs(fmt.Sprintf("/entities/%s/relationships/%d/role", name, i), rel.Role)
			checkRefs(fmt.Sprintf("/entities/%s/relationships/%d/note", name, i), rel.Note)
		}
		for i, attr := range ent.Attributes {
			// A PascalCase type is an enum reference; it must resolve.
			if pascalCaseRE.MatchString(attr.Type) && !enums[attr.Type] {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityWarning,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/attributes/%d/type", name, i),
					Message:  fmt.Sprintf("attribute type %q looks like an enum reference but no enum %q is defined (primitives are lowercase)", attr.Type, attr.Type),
				})
			}
			checkRefs(fmt.Sprintf("/entities/%s/attributes/%d/derivation", name, i), attr.Derivation)
		}
		for i, act := range ent.Actions {
			if act.Actor != "" && !entitySet[act.Actor] && !glossary[act.Actor] {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityWarning,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/actions/%d/actor", name, i),
					Message:  fmt.Sprintf("action actor %q is not a defined entity or glossary term", act.Actor),
				})
			}
			for j, id := range act.Preserves {
				if !invariantID[id] {
					res.Findings = append(res.Findings, Finding{
						Severity: SeverityError,
						Category: CategorySemantic,
						Path:     fmt.Sprintf("/entities/%s/actions/%d/preserves/%d", name, i, j),
						Message:  fmt.Sprintf("action preserves unknown invariant id %q", id),
					})
				}
			}
			checkRefs(fmt.Sprintf("/entities/%s/actions/%d/description", name, i), act.Description)
		}
		for i, inv := range ent.Invariants {
			checkRefs(fmt.Sprintf("/entities/%s/invariants/%d/statement", name, i), inv.Statement)
		}
	}
	for i, inv := range m.Invariants {
		checkRefs(fmt.Sprintf("/invariants/%d/statement", i), inv.Statement)
	}
	for i, sc := range m.Scenarios {
		for j, step := range sc.Steps {
			checkRefs(fmt.Sprintf("/scenarios/%d/steps/%d", i, j), step)
		}
	}

	// Scenario invariants_touched must reference a declared invariant id. With
	// stable ids (DDD-9) this is a real reference, so a miss is a broken
	// reference (error), not the soft "may be a gap" signal it was as free text.
	for i, sc := range m.Scenarios {
		for j, id := range sc.InvariantsTouched {
			if !invariantID[strings.TrimSpace(id)] {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/scenarios/%d/invariants_touched/%d", i, j),
					Message:  fmt.Sprintf("scenario %q touches unknown invariant id %q", sc.Name, id),
				})
			}
		}
	}
}

// runRelationshipShape checks each relationship's cardinality and symmetric
// marker beyond what the schema pattern can express. The schema pattern accepts
// an inverted range like "5..2"; here it becomes a semantic error. A symmetric
// marker is only meaningful when the two ends are interchangeable, so it is
// restricted to a self-referential relationship or one whose target side is
// more than one.
func runRelationshipShape(m *model.Model, res *Result) {
	for _, name := range m.EntityNames() {
		for i, rel := range m.Entities[name].Relationships {
			path := fmt.Sprintf("/entities/%s/relationships/%d", name, i)

			// The schema pattern accepts an inverted range like "5..2"
			// syntactically; flag only that semantic case here. Other parse
			// failures (a malformed string, an absurd overflow) are the
			// schema's to report, so this check doesn't double up on them.
			if hasInvertedRange(rel.Cardinality) {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     path + "/cardinality",
					Message: fmt.Sprintf(
						"invalid cardinality %q: a range's minimum must not exceed its maximum",
						rel.Cardinality,
					),
				})
			}

			// A symmetric marker is only meaningful when the two ends are
			// interchangeable. Skip the check when the cardinality doesn't parse
			// — the invalid cardinality is the finding to fix first, and the
			// target side can't be judged.
			_, right, ok := model.ParseCardinality(rel.Cardinality)
			if rel.Symmetric && ok {
				selfReferential := rel.Entity == name
				targetIsMany := right.Max < 0 || right.Max > 1
				if !selfReferential && !targetIsMany {
					res.Findings = append(res.Findings, Finding{
						Severity: SeverityError,
						Category: CategorySemantic,
						Path:     path + "/symmetric",
						Message: fmt.Sprintf(
							"symmetric relationship %s→%s must be self-referential or have a target side greater than one",
							name, rel.Entity,
						),
					})
				}
			}
		}
	}
}

// hasInvertedRange reports whether either side of a cardinality is a range whose
// minimum exceeds its maximum (e.g. "5..2"). This is the one semantic error the
// schema's syntactic pattern cannot catch.
func hasInvertedRange(card string) bool {
	a, b, ok := strings.Cut(card, ":")
	if !ok {
		return false
	}
	return sideInverted(a) || sideInverted(b)
}

func sideInverted(s string) bool {
	lo, hi, isRange := strings.Cut(s, "..")
	if !isRange || hi == "n" {
		return false
	}
	min, err1 := strconv.Atoi(lo)
	max, err2 := strconv.Atoi(hi)
	return err1 == nil && err2 == nil && min > max
}

// runReciprocity checks that a relationship declared from both sides agrees:
// B→A must declare the inverse of A→B's cardinality. A contradiction (e.g. A
// says "1:n" B while B says "1:1" A) is an error — the model can't be both, and
// the renderer would otherwise draw two conflicting edges.
//
// Only pairs with exactly one declaration in each direction are checked.
// Multiple edges between the same pair (a legitimate pattern — e.g. a User is
// both `Owner` and `Member` of a Project) can't be paired up unambiguously, so
// they're left alone rather than guessed at.
func runReciprocity(m *model.Model, res *Result) {
	type decl struct {
		from, to string
		card     string
		path     string
	}
	byPair := map[string][]decl{}
	for _, name := range m.EntityNames() {
		for i, rel := range m.Entities[name].Relationships {
			pair := []string{name, rel.Entity}
			sort.Strings(pair)
			k := pair[0] + "\x00" + pair[1]
			byPair[k] = append(byPair[k], decl{
				from: name,
				to:   rel.Entity,
				card: rel.Cardinality,
				path: fmt.Sprintf("/entities/%s/relationships/%d/cardinality", name, i),
			})
		}
	}

	for _, k := range sortedMapKeys(byPair) {
		decls := byPair[k]
		var fwd, rev []decl // fwd: from == sorted pair[0]; rev: from == pair[1]
		for _, d := range decls {
			switch {
			case d.from == d.to:
				// Self-relationship; no reciprocal to reconcile.
			case d.from < d.to:
				fwd = append(fwd, d)
			default:
				rev = append(rev, d)
			}
		}
		if len(fwd) != 1 || len(rev) != 1 {
			continue
		}
		f, r := fwd[0], rev[0]
		// Reciprocity is checked only for structurally valid cardinalities; an
		// invalid one is already reported (by the schema, and by
		// runRelationshipShape). Compare the parsed multiplicities, not the raw
		// strings, so semantically equal declarations written differently
		// ("1:n" one way, "0..n:1" the other) don't read as a conflict.
		fL, fR, fok := model.ParseCardinality(f.card)
		rL, rR, rok := model.ParseCardinality(r.card)
		if !fok || !rok {
			continue
		}
		if fL != rR || fR != rL {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     f.path,
				Message: fmt.Sprintf(
					"reciprocal cardinality conflict: %s→%s declares %q but %s→%s declares %q (expected %q, the inverse)",
					f.from, f.to, f.card, r.from, r.to, r.card, model.InvertCardinality(f.card),
				),
			})
		}
	}
}

func runCompleteness(m *model.Model, res *Result) {
	// Entities with no invariants.
	for _, name := range m.EntityNames() {
		if len(m.Entities[name].Invariants) == 0 {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarning,
				Category: CategoryCompleteness,
				Path:     fmt.Sprintf("/entities/%s", name),
				Message:  fmt.Sprintf("entity %q has no invariants — fine if no rule must always hold for it, otherwise the rules that govern it are worth capturing", name),
			})
		}
	}

	// Entities no scenario exercises.
	referenced := map[string]bool{}
	canonical := map[string]string{}
	for name := range m.Entities {
		canonical[name] = name
		canonical[plural(name)] = name
	}
	mark := func(token string) {
		if c, ok := canonical[token]; ok {
			referenced[c] = true
		}
	}
	for _, sc := range m.Scenarios {
		for _, actor := range sc.Actors {
			mark(strings.TrimSpace(actor))
		}
		for _, step := range sc.Steps {
			for _, base := range entityRefs(step) {
				mark(base)
			}
		}
	}
	for _, name := range m.EntityNames() {
		if !referenced[name] {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarning,
				Category: CategoryCompleteness,
				Path:     fmt.Sprintf("/entities/%s", name),
				Message:  fmt.Sprintf("no scenario exercises entity %q", name),
			})
		}
	}

	// Defined-but-unused glossary terms and enums — vocabulary nothing references.
	usedTerm := map[string]bool{}
	scan := func(text string) {
		for _, b := range entityRefs(text) {
			usedTerm[b] = true
		}
	}
	usedEnum := map[string]bool{}
	for _, def := range m.Glossary {
		scan(def)
	}
	for _, en := range m.Enums {
		scan(en.Description)
		for _, v := range en.Values {
			scan(v.Definition)
		}
	}
	for _, ent := range m.Entities {
		scan(ent.Definition)
		for _, rel := range ent.Relationships {
			scan(rel.Role)
			scan(rel.Note)
		}
		for _, attr := range ent.Attributes {
			scan(attr.Derivation)
			if attr.Type != "" {
				usedEnum[attr.Type] = true
			}
		}
		for _, act := range ent.Actions {
			if actor := strings.TrimSpace(act.Actor); actor != "" {
				usedTerm[actor] = true
			}
			scan(act.Description)
		}
		for _, inv := range ent.Invariants {
			scan(inv.Statement)
		}
	}
	for _, inv := range m.Invariants {
		scan(inv.Statement)
	}
	for _, sc := range m.Scenarios {
		for _, actor := range sc.Actors {
			usedTerm[strings.TrimSpace(actor)] = true
		}
		for _, step := range sc.Steps {
			scan(step)
		}
	}
	for _, term := range sortedMapKeys(m.Glossary) {
		if !usedTerm[term] {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarning,
				Category: CategoryCompleteness,
				Path:     fmt.Sprintf("/glossary/%s", term),
				Message:  fmt.Sprintf("glossary term %q is defined but never referenced", term),
			})
		}
	}
	for _, name := range sortedMapKeys(m.Enums) {
		if !usedEnum[name] {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarning,
				Category: CategoryCompleteness,
				Path:     fmt.Sprintf("/enums/%s", name),
				Message:  fmt.Sprintf("enum %q is defined but no attribute uses it", name),
			})
		}
	}
}

// sortedMapKeys returns a map's string keys in stable order, so iteration that
// emits findings is deterministic.
func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// entityRefs extracts backticked, entity-looking terms from freeform text. A
// term qualifies if, after stripping a trailing ".field" accessor, it is
// PascalCase. Lowercase terms (field names, types) are ignored.
func entityRefs(text string) []string {
	var out []string
	for _, m := range backtickRE.FindAllStringSubmatch(text, -1) {
		tok := m[1]
		if i := strings.IndexByte(tok, '.'); i >= 0 {
			tok = tok[:i]
		}
		tok = strings.TrimSpace(tok)
		if pascalCaseRE.MatchString(tok) {
			out = append(out, tok)
		}
	}
	return out
}

// plural is a naive English pluralizer good enough to match entity names like
// Policy -> Policies and Project -> Projects.
func plural(s string) string {
	switch {
	case strings.HasSuffix(s, "y"):
		return s[:len(s)-1] + "ies"
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	default:
		return s + "s"
	}
}

// sortFindings orders findings so the most actionable surface first: errors
// before warnings, then by layer (structural → semantic → completeness), then
// by path and message for determinism.
func sortFindings(res *Result) {
	severityRank := map[Severity]int{SeverityError: 0, SeverityWarning: 1}
	categoryRank := map[Category]int{CategoryStructural: 0, CategorySemantic: 1, CategoryCompleteness: 2}
	sort.SliceStable(res.Findings, func(i, j int) bool {
		a, b := res.Findings[i], res.Findings[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if categoryRank[a.Category] != categoryRank[b.Category] {
			return categoryRank[a.Category] < categoryRank[b.Category]
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Message < b.Message
	})
}
