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

// Run validates the model at path and returns all findings. src is the model's
// own bytes; files reads the ones its `imports:` name, resolved relative to
// path's directory, and answers where that resolution is confined. A nil files
// uses the local filesystem.
func Run(path string, src []byte, files Files) (*Result, error) {
	res := &Result{}
	if files == nil {
		files = OSFiles{}
	}

	// Whether this file is somebody else's copy is settled first: it decides
	// which of the layers below are findings about a model this repository
	// controls, and it holds even when the file does not parse.
	vendored := runProvenance(src, res)

	// Layer 1: structural validation against the JSON Schema.
	structuralOK, entityScopes := runStructural(src, res)

	// If it does not even parse into our typed model, stop — semantic and
	// completeness checks need a model to work with. The structural layer has
	// already reported why.
	m, err := model.Parse(src)
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
		if vendored {
			dropOwnedDiagnostics(res)
		}
		sortFindings(res)
		return res, nil
	}

	runSemantic(m, res)
	// Imports resolve only against a document the schema accepted. A scope the
	// schema already rejected would otherwise bind anyway, and the advice that
	// follows would tell the author to write syntax that cannot work — the same
	// reason the version check gates schema validation. A cross-model reference
	// in an entity position is not one of those rejections (see runStructural),
	// so it does not take the imports layer down with it.
	if structuralOK {
		runImports(path, m, files, res, entityScopes, vendored)
	}
	runRelationshipShape(m, res)
	runSubtypes(m, res)
	runReciprocity(m, res)
	runPairing(m, res)
	runCompleteness(m, res)

	if vendored {
		dropOwnedDiagnostics(res)
	}
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

// runStructural validates against the JSON Schema. Returns true if the schema
// accepted the document — which the cross-model entity references reported here
// do not affect, since they are a supported-feature limit rather than a shape
// the imports list depends on — plus the scopes those references named, so the
// imports layer can tell an import that exists to support one of them from one
// genuinely unreferenced (runSemantic and runSubtypes rely on every such
// reference having been reported here; see reportQualifiedEntityRefs).
func runStructural(data []byte, res *Result) (ok bool, entityScopes map[string]bool) {
	jsonBytes, err := yaml.YAMLToJSON(data)
	if err != nil {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  fmt.Sprintf("not valid YAML: %v", err),
		})
		return false, nil
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  fmt.Sprintf("could not decode document: %v", err),
		})
		return false, nil
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
				// A cross-model entity reference is reported here regardless of
				// whether the version is one this build understands: runSemantic and
				// runSubtypes skip it on the assumption it was, and an early return
				// before this call would leave it unreported instead of just
				// unvalidated.
				_, entityScopes = reportQualifiedEntityRefs(inst, res)
				return false, entityScopes
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
		return false, nil
	}

	// Say what a cross-model entity reference actually is before the schema
	// reports it as a pattern violation, and suppress its opaque message so one
	// mistake reads as one finding. It is reported on its own terms and is not
	// counted against the document's structural validity: a broken import in the
	// same file is an unrelated mistake, and holding the imports layer back
	// until this one is fixed would hide it.
	qualified, entityScopes := reportQualifiedEntityRefs(inst, res)

	before := len(res.Findings)
	if err := sch.Validate(inst); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			collectLeaves(ve, res, qualified)
			return len(res.Findings) == before, entityScopes
		}
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Message:  err.Error(),
		})
		return false, entityScopes
	}
	return true, entityScopes
}

func collectLeaves(e *jsonschema.ValidationError, res *Result, skip map[string]bool) {
	if len(e.Causes) == 0 {
		ptr := "/" + strings.Join(e.InstanceLocation, "/")
		if ptr == "/" {
			ptr = ""
		}
		if skip[ptr] {
			return
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
		collectLeaves(c, res, skip)
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
			switch {
			case qualifiedRefRE.MatchString(rel.Entity):
				// Already reported as an unsupported cross-model reference by
				// reportQualifiedEntityRefs; calling it an undefined entity too
				// would report one mistake twice.
			case !entitySet[rel.Entity]:
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/relationships/%d/entity", name, i),
					Message:  fmt.Sprintf("relationship targets undefined entity %q", rel.Entity),
				})
			case rel.Ownership == "owned" && m.Entities[rel.Entity].Derived:
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityWarning,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/relationships/%d/ownership", name, i),
					Message:  fmt.Sprintf("entity %q owns %q, which is derived — composing an ephemeral, never-persisted entity is usually a modeling error", name, rel.Entity),
				})
			}
			// A prose role wrecks the diagram: it is the only label on the
			// rendered relationship line (ADR-0008), so a sentence there
			// collides with its neighbours. `note` is the field for prose.
			//
			// One role raises one finding. Rewriting a prose role is the fix
			// that comes first, and the terms buried in the sentence are likely
			// to change with it, so the undefined-term check waits until the
			// role is a role name.
			if readsAsProse(rel.Role) {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityWarning,
					Category: CategorySemantic,
					Path:     fmt.Sprintf("/entities/%s/relationships/%d/role", name, i),
					Message: fmt.Sprintf(
						"role %q reads as prose — keep the role to a short role name (ideally a glossary term) and move the explanation to the relationship's note",
						rel.Role,
					),
				})
			} else {
				// A role names a non-entity vocabulary term; it should resolve
				// to an entity or a glossary term (the DDD-1 payoff — undefined
				// roles).
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
		checkRefs(fmt.Sprintf("/entities/%s/derivation", name), ent.Derivation)
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

// runSubtypes validates the is-a links: a subtypeOf must name a defined entity,
// and the chain of subtypeOf links must not cycle (an entity cannot be a kind
// of itself, directly or transitively).
func runSubtypes(m *model.Model, res *Result) {
	for _, name := range m.EntityNames() {
		parent := m.Entities[name].SubtypeOf
		if parent == "" {
			continue
		}
		if qualifiedRefRE.MatchString(parent) {
			continue // reported as an unsupported cross-model reference
		}
		if _, ok := m.Entities[parent]; !ok {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     fmt.Sprintf("/entities/%s/subtypeOf", name),
				Message:  fmt.Sprintf("entity %q is a subtype of undefined entity %q", name, parent),
			})
			continue
		}
		if subtypeChainCycles(m, name) {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     fmt.Sprintf("/entities/%s/subtypeOf", name),
				Message:  fmt.Sprintf("entity %q has a cyclic subtypeOf chain: an entity cannot be a kind of itself, directly or transitively", name),
			})
		}
	}
}

// subtypeChainCycles reports whether following name's subtypeOf links revisits
// an entity, which means name is (transitively) a subtype of itself. It stops
// at an undefined parent, which is reported separately.
func subtypeChainCycles(m *model.Model, name string) bool {
	seen := map[string]bool{}
	for cur := name; cur != ""; {
		e, ok := m.Entities[cur]
		if !ok {
			return false
		}
		if seen[cur] {
			return true
		}
		seen[cur] = true
		cur = e.SubtypeOf
	}
	return false
}

// inheritsInvariants reports whether any ancestor of name (via subtypeOf) has an
// invariant, so a subtype's own empty invariant list is covered by its parent.
// The walk is cycle-safe and stops at an undefined parent.
func inheritsInvariants(m *model.Model, name string) bool {
	seen := map[string]bool{name: true}
	for cur := m.Entities[name].SubtypeOf; cur != "" && !seen[cur]; {
		e, ok := m.Entities[cur]
		if !ok {
			return false
		}
		if len(e.Invariants) > 0 {
			return true
		}
		seen[cur] = true
		cur = e.SubtypeOf
	}
	return false
}

// runReciprocity checks that a relationship declared from both sides agrees, on
// cardinality and on ownership.
//
// B→A must declare the inverse of A→B's cardinality. A contradiction (e.g. A
// says "1:n" B while B says "1:1" A) is an error — the model can't be both, and
// the renderer would otherwise draw two conflicting edges.
//
// Ownership belongs to the relationship, not to the end that declared it, so at
// most one end may claim `owned`; both claiming it is a contradiction. One end
// claiming it is the ordinary composition pattern — the renderer folds the two
// declarations into a single identifying line (ADR-0008), whatever roles the
// two ends give it.
//
// Only pairs with exactly one declaration in each direction are checked.
// Multiple edges between the same pair (a legitimate pattern — e.g. a User is
// both `Owner` and `Member` of a Project) can't be paired up unambiguously, so
// they're left alone rather than guessed at.
func runReciprocity(m *model.Model, res *Result) {
	type decl struct {
		from, to  string
		card      string
		owned     bool
		path      string
		ownerPath string
	}
	byPair := map[string][]decl{}
	for _, name := range m.EntityNames() {
		for i, rel := range m.Entities[name].Relationships {
			pair := []string{name, rel.Entity}
			sort.Strings(pair)
			k := pair[0] + "\x00" + pair[1]
			byPair[k] = append(byPair[k], decl{
				from:      name,
				to:        rel.Entity,
				card:      rel.Cardinality,
				owned:     rel.Ownership == "owned",
				path:      fmt.Sprintf("/entities/%s/relationships/%d/cardinality", name, i),
				ownerPath: fmt.Sprintf("/entities/%s/relationships/%d/ownership", name, i),
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

		// Mutual ownership is a contradiction whatever the cardinalities say,
		// so it is checked before they are parsed.
		if f.owned && r.owned {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     f.ownerPath,
				Message: fmt.Sprintf(
					"mutual ownership: %s→%s and %s→%s both declare ownership \"owned\" — a relationship is owned by at most one end, so make the other end \"referenced\" (or omit it)",
					f.from, f.to, r.from, r.to,
				),
			})
		}

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

// runPairing warns where a relationship declared from both ends cannot be
// paired up: one end declares the same line more than once, so which
// declaration is the reciprocal of which is undeterminable — the format has no
// way to say. The renderer refuses to guess and draws every declaration as its
// own line (ADR-0008), which is lossless but shows more lines than the author
// probably means, so it is worth naming.
//
// A warning, never an error: the model may be exactly right, and there is no
// wording of it the linter could demand instead. Declaring each relationship
// from one end only resolves it.
func runPairing(m *model.Model, res *Result) {
	for _, g := range model.EdgeGroups(m) {
		if !g.AmbiguousPairing() {
			continue
		}
		// Point at the first declaration on the crowded end — the one an author
		// would edit — preferring the alphabetically earlier entity when both
		// ends are crowded, so the finding is stable.
		at := g.First
		if len(at) < 2 && len(g.Second) > 1 {
			at = g.Second
		}
		d := at[0]
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityWarning,
			Category: CategorySemantic,
			Path:     fmt.Sprintf("/entities/%s/relationships/%d", d.From, d.Index),
			Message: fmt.Sprintf(
				"ambiguous reciprocal pairing between %q and %q at cardinality %q: %s declares it %d time(s) and %s %d time(s), so which declaration is the reciprocal of which cannot be determined — the diagram draws every declaration as its own line; declare each relationship from one end only to pair them up",
				g.FirstN, g.SecondN, g.First[0].Rel.Cardinality,
				g.FirstN, len(g.First), g.SecondN, len(g.Second),
			),
		})
	}
}

func runCompleteness(m *model.Model, res *Result) {
	// Entities with no invariants — unless a supertype's invariants cover them.
	for _, name := range m.EntityNames() {
		if len(m.Entities[name].Invariants) == 0 && !inheritsInvariants(m, name) {
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
	//
	// A shared model is imported by others, and an incoming reference is
	// invisible from here: the vocabulary such a model exists to publish would
	// otherwise read as vocabulary nothing uses, and a pure vocabulary model
	// could never pass --completeness error. Only these two checks are relaxed;
	// a missing invariant or an unexercised entity is a gap in the model itself,
	// which being imported does not fill.
	if m.Shared {
		return
	}

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
		scan(ent.Derivation)
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

// roleLabelMax is the longest a role may be before it crowds the diagram line
// it labels, whatever its shape. Measured in runes, so a non-ASCII role is not
// judged by its byte count.
const roleLabelMax = 40

// readsAsProse reports whether a relationship role is written as a sentence
// rather than a role name. It judges the role as the diagram will draw it, so
// backticks — markup, not label text — come off first.
//
// Three signals, each of which alone wrecks a diagram label: too long, too many
// words, or a sentence terminator. Sentence punctuation is deliberately *not*
// tested character-by-character: a comma separates a short list of role names
// ("`Owner`, `Member`"), and a full stop is as likely to be an accessor
// ("`Project`.owner") or a version ("v1.0 owner") as the end of a sentence.
// "`Owner` or `Member`" passes; "the record this one supersedes" does not.
func readsAsProse(role string) bool {
	role = model.NormalizeRole(role)
	if role == "" {
		return false
	}
	if len([]rune(role)) > roleLabelMax {
		return true
	}
	if strings.Contains(role, ";") || strings.HasSuffix(role, ".") {
		return true
	}
	return len(strings.Fields(role)) > 4
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
