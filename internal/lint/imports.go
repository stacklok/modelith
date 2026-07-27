package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/schema"
)

// Files is the filesystem seam import resolution runs on: reading an imported
// model, and the two path questions that decide whether it may be read at all.
// Reading and containment sit behind one interface deliberately — judging the
// boundary against the local disk while reading from somewhere else would
// confine imports on a filesystem the reads never touch.
//
// It is not an fs.FS: fs.ValidPath rejects "..", and peer models commonly sit
// in sibling directories, so "../payments/payments.modelith.yaml" has to work.
type Files interface {
	// ReadFile reads the named file.
	ReadFile(path string) ([]byte, error)
	// ResolutionRoot returns the directory an import of the model at modelPath
	// may not resolve outside of, and whether an enclosing repository defined
	// it (ADR-0013).
	ResolutionRoot(modelPath string) (root string, inRepo bool)
	// Resolve returns path as this filesystem sees it — absolute, cleaned, and
	// symlink-resolved as far as it can be — so containment judges where a path
	// lands rather than how it was written. It must agree with ResolutionRoot:
	// the two results are compared against each other.
	Resolve(path string) string
}

// OSFiles resolves imported models against the local filesystem. Its path
// methods are in root.go, alongside the containment rules they implement.
type OSFiles struct{}

// ReadFile reads the named file from the local filesystem.
func (OSFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

var (
	// qualifiedRefRE matches a well-formed cross-model reference, "scope.Name":
	// exactly one dot, a scope slug before it, a PascalCase item name after.
	// Anything else containing a dot is a malformed reference, not a primitive
	// type — malformedRefReason says which way it is malformed.
	qualifiedRefRE = regexp.MustCompile(`^(` + model.ScopeSlug + `)\.([A-Z][A-Za-z0-9]*)$`)
	// scopeRE is the slug a bare import's filename has to yield. An explicitly
	// written scope is held to the same pattern by the schema.
	scopeRE = regexp.MustCompile(`^` + model.ScopeSlug + `$`)
)

// importedModel is one successfully resolved entry of a model's imports list.
type importedModel struct {
	index int    // position in the importing model's imports list, for the finding path
	path  string // the path as written in imports
	model *model.Model
}

// runImports resolves the model's imports, checks every qualified attribute
// type against them, and reports an import nothing references.
//
// modelPath is the path of the model being linted; imports resolve relative to
// its directory. entityScopes are the scopes named by a cross-model reference
// in an entity position (relationship.entity, subtypeOf) — unsupported there,
// but still a real reference: an import bound to one of them is not also
// reported as unreferenced (see reportQualifiedEntityRefs).
//
// vendored says the model is a copy whose home is another repository, which
// silences the errors its imports list would raise here (see loadImports).
func runImports(modelPath string, m *model.Model, files Files, res *Result, entityScopes map[string]bool, vendored bool) {
	byScope, claimed := loadImports(modelPath, m, files, res, vendored)
	used := checkQualifiedTypes(m, byScope, claimed, res)
	// An unreferenced import is a completeness finding, alongside the unused
	// enum and the unused glossary term: vocabulary the model declares and
	// nothing uses. Sharing their category means sharing their promotion under
	// --completeness error.
	for _, scope := range sortedMapKeys(byScope) {
		if used[scope] || entityScopes[scope] {
			continue
		}
		imp := byScope[scope]
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityWarning,
			Category: CategoryCompleteness,
			Path:     fmt.Sprintf("/imports/%d", imp.index),
			Message: fmt.Sprintf(
				"import %q (scope %q) is never referenced — drop it, or reference one of its enums as %s.Name in an attribute type",
				imp.path, scope, scope,
			),
		})
	}
}

// loadImports reads each declared import and returns the ones that resolved,
// keyed by the scope the importing model binds them to, plus every scope the
// list claimed at all — including entries that then failed to load — mapped to
// the path of the first entry that claimed it. Every rejection is reported as an
// error: an import that cannot be resolved is a broken reference, not a gap.
//
// A vendored model is the exception, and it reports nothing here at all. Its
// imports name paths in the repository it came from, which do not exist in this
// one, so every entry would fail for a reason its authors cannot see and this
// repository is not expected to fix. Resolution is still attempted, because two
// vendored peers that import each other do resolve here; and every entry still
// claims its scope, which is what keeps a reference into an import that did not
// load from reading as a reference to nothing (see checkQualifiedTypes).
func loadImports(modelPath string, m *model.Model, files Files, res *Result, vendored bool) (byScope map[string]importedModel, claimed map[string]string) {
	byScope = map[string]importedModel{}
	claimed = map[string]string{}
	if len(m.Imports) == 0 {
		return byScope, claimed
	}
	dir := filepath.Dir(modelPath)
	root, inRepo := files.ResolutionRoot(modelPath)
	for i, imp := range m.Imports {
		reject := func(format string, args ...any) {
			if vendored {
				return
			}
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     fmt.Sprintf("/imports/%d", i),
				Message:  fmt.Sprintf(format, args...),
			})
		}
		if imp.Path == "" {
			continue // the schema's minLength reports the empty path
		}
		// The scope is settled before the path is judged, and claimed before the
		// file is opened. Reference sites depend on the scope alone, so an entry
		// that names a usable one holds it whatever else is wrong with the entry:
		// a duplicate binding is then reported on its own terms instead of being
		// pre-empted by the second entry's unrelated trouble, and a broken path
		// does not also make every "scope.Name" that names it look like a
		// reference to nothing. A written scope is held to this same pattern by
		// the schema, which runs before this layer and gates it, so a scope that
		// fails here was derived from the filename — which is what the advice
		// below assumes. TestInvariant_ScopeSlugMatchesSchema guards the two.
		scopeOK := scopeRE.MatchString(imp.Scope)
		if scopeOK {
			if prev, dup := claimed[imp.Scope]; dup {
				reject("import %q binds scope %q, which import %q already binds — give one of them an explicit, different scope so %s.Name resolves unambiguously",
					imp.Path, imp.Scope, prev, imp.Scope)
				continue
			}
			claimed[imp.Scope] = imp.Path
		}
		// A control character is never part of a filename, and one that reached
		// here would be carried into this diagnostic, the shell, and the rendered
		// Markdown. Every message here quotes the path with %q, so an escape
		// survives as text rather than as a line break in the output.
		if at := strings.IndexFunc(imp.Path, unicode.IsControl); at >= 0 {
			reject("import path %q contains a control character at byte %d — a filename cannot contain one; check for a stray newline or escape in the YAML", imp.Path, at)
			continue
		}
		if !scopeOK {
			// A filename that yields no usable slug is the case the explicit form
			// exists for.
			reject("import %q binds the scope %q taken from its filename, which is not a valid slug (lowercase kebab-case) — name the scope explicitly instead: `- {scope: <slug>, path: %q}`",
				imp.Path, imp.Scope, imp.Path)
			continue
		}
		if filepath.IsAbs(imp.Path) || imp.Path[0] == '/' {
			reject("import %q is an absolute path — imports are relative to this model so they resolve in any checkout", imp.Path)
			continue
		}
		// Containment is decided before the file is opened, because opening it
		// is the leak: whether a path resolves, is missing, is unreadable, or
		// holds no model are four distinct diagnostics, and together they let a
		// model from an untrusted source probe the filesystem of whatever runner
		// lints it (ADR-0013).
		joined := filepath.Join(dir, imp.Path)
		if resolved := files.Resolve(joined); !withinRoot(root, resolved) {
			if inRepo {
				reject("import %q resolves to %q, outside %q — that directory is the repository holding this model (the nearest ancestor with a .git entry), and an import may not name a file beyond it",
					imp.Path, resolved, root)
			} else {
				reject("import %q resolves to %q, outside %q — this model is in no repository, so resolution is confined to the directory holding it; move the imported model into that directory or below it",
					imp.Path, resolved, root)
			}
			continue
		}
		data, err := files.ReadFile(joined)
		if err != nil {
			reject("import %q cannot be read: %v", imp.Path, err)
			continue
		}
		im, err := model.Parse(data)
		if err != nil || im.Kind != "DomainModel" {
			reject("import %q is not a domain model — lint it on its own with `modelith lint` to see why", imp.Path)
			continue
		}
		if !schema.Supported(im.Version) {
			reject("import %q declares schema version %q, which this modelith does not support: %s (upgrade modelith, or move that model to a supported version)",
				imp.Path, im.Version, strings.Join(schema.SupportedVersions(), ", "))
			continue
		}
		byScope[imp.Scope] = importedModel{index: i, path: imp.Path, model: im}
	}
	return byScope, claimed
}

// checkQualifiedTypes resolves every qualified attribute type against the
// imports and returns the set of scopes that were referenced.
//
// A qualified type that does not resolve is an error, where an unqualified
// PascalCase type that names no enum is only a warning (runSemantic). The
// asymmetry is deliberate: an unqualified name may be a primitive the author
// invented, while "scope.Name" can only be a cross-model reference.
func checkQualifiedTypes(m *model.Model, byScope map[string]importedModel, claimed map[string]string, res *Result) map[string]bool {
	used := map[string]bool{}
	unbound := map[string]*unboundScope{}
	for _, name := range m.EntityNames() {
		for i, attr := range m.Entities[name].Attributes {
			// A dot in a type can only be a cross-model reference: primitives are
			// lowercase words and enum names are PascalCase, neither of which
			// carries one. So a dotted type that is not well formed is a typo to
			// report, not a type to pass over.
			if !strings.Contains(attr.Type, ".") {
				continue
			}
			path := fmt.Sprintf("/entities/%s/attributes/%d/type", name, i)
			broken := func(format string, args ...any) {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     path,
					Message:  fmt.Sprintf(format, args...),
				})
			}
			match := qualifiedRefRE.FindStringSubmatch(attr.Type)
			if match == nil {
				broken("attribute type %q is a malformed cross-model reference (%s) — write it as scope.Name: one dot, a lowercase kebab-case scope, a PascalCase item name",
					attr.Type, malformedRefReason(attr.Type))
				// The scope the author was reaching for is still referenced, so a
				// typo in the item name does not also read as an unused import.
				if head, _, _ := strings.Cut(attr.Type, "."); byScope[head].model != nil {
					used[head] = true
				}
				continue
			}
			scope, item := match[1], match[2]
			imp, ok := byScope[scope]
			if !ok {
				if _, listed := claimed[scope]; listed {
					// An import claims this scope but failed to load, and that
					// failure is already reported against the imports list. It is
					// the one thing to fix; saying the scope is unbound here would
					// report the same mistake again at every reference site.
					used[scope] = true
					continue
				}
				u, seen := unbound[scope]
				if !seen {
					unbound[scope] = &unboundScope{path: path, typ: attr.Type, item: item, count: 1}
					continue
				}
				u.count++
				continue
			}
			// The import is referenced even when the item does not resolve, so
			// the same mistake never reads as an unused import too.
			used[scope] = true
			if _, ok := imp.model.Enums[item]; ok {
				continue
			}
			if _, ok := imp.model.Entities[item]; ok {
				broken("attribute type %q resolves to the entity %q in %q — only an enum can be referenced across models", attr.Type, item, imp.path)
				continue
			}
			broken("%s", unresolvedItemMessage(attr.Type, item, imp))
		}
	}
	reportUnboundScopes(unbound, res)
	return used
}

// unresolvedItemMessage explains a qualified type whose scope resolved but
// whose item is not there.
//
// The interesting case is an imported model that has imports of its own: the
// item may well be defined in one of them, and nothing about the reference site
// says why that does not resolve. Vendoring fetches one file and resolution
// reaches one hop (ADR-0010, ADR-0015), so the answer is always to import that
// model here as well and give it its own scope — never to expect this one to
// follow the chain. Saying so only when the imported model actually has imports
// keeps the advice off the plain case, where the mistake is a typo.
func unresolvedItemMessage(typ, item string, imp importedModel) string {
	msg := fmt.Sprintf("attribute type %q names no enum %q in %q", typ, item, imp.path)
	n := len(imp.model.Imports)
	if n == 0 {
		return msg + " — check the name, or whether you meant to import a different model"
	}
	theirs := fmt.Sprintf("%d models of its own", n)
	if n == 1 {
		theirs = "a model of its own"
	}
	return msg + fmt.Sprintf(
		" — that model imports %s, and resolution is not transitive: if %q is defined in one of them, add that model to this model's `imports:` too and reference it with its own scope",
		theirs, item)
}

// unboundScope accumulates the references to one scope no import binds, so a
// single missing import is reported once rather than once per reference site.
type unboundScope struct {
	path  string // where the first reference is
	typ   string // the first reference, verbatim
	item  string
	count int
}

func reportUnboundScopes(unbound map[string]*unboundScope, res *Result) {
	for _, scope := range sortedMapKeys(unbound) {
		u := unbound[scope]
		msg := fmt.Sprintf("attribute type %q references the scope %q, which no import binds — add the model that defines %s to `imports:`", u.typ, scope, u.item)
		if u.count > 1 {
			msg += fmt.Sprintf(" (%d attribute types reference this scope; this is the first)", u.count)
		}
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategorySemantic,
			Path:     u.path,
			Message:  msg,
		})
	}
}

// malformedRefReason names what is wrong with a type that contains a dot but is
// not a well-formed "scope.Name", so the diagnostic points at the fix instead of
// only restating the shape.
func malformedRefReason(typ string) string {
	scope, item, _ := strings.Cut(typ, ".")
	switch {
	case scope == "":
		return "nothing before the dot"
	case item == "":
		return "nothing after the dot"
	case strings.Contains(item, "."):
		return "more than one dot"
	case !scopeRE.MatchString(scope):
		return fmt.Sprintf("the scope %q is not lowercase kebab-case", scope)
	default:
		return fmt.Sprintf("the item name %q is not PascalCase", item)
	}
}

// reportQualifiedEntityRefs reports a cross-model reference in an entity
// position — relationship.entity or subtypeOf. It returns the instance paths
// it reported, so the schema's own finding for the same value is suppressed,
// and the scopes those references named, so an import that exists to support
// one of them is not also reported as unreferenced (runImports) even though no
// attribute type resolves it.
//
// Both fields carry pattern ^[A-Z][A-Za-z0-9]+$, so "payments.Card" already
// fails validation with a message about a pattern. This says what is actually
// wrong, in the spirit of the unsupported-version check. Cross-model entity
// references are deferred, not planned against: ADR-0010 records why.
func reportQualifiedEntityRefs(inst any, res *Result) (reported map[string]bool, scopes map[string]bool) {
	reported = map[string]bool{}
	scopes = map[string]bool{}
	doc, ok := inst.(map[string]any)
	if !ok {
		return reported, scopes
	}
	entities, ok := doc["entities"].(map[string]any)
	if !ok {
		return reported, scopes
	}
	report := func(path, value string) {
		reported[path] = true
		scope, _, _ := strings.Cut(value, ".")
		scopes[scope] = true
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategoryStructural,
			Path:     path,
			Message: fmt.Sprintf(
				"%q is a cross-model reference, which is not supported in an entity position — only an attribute `type` can be qualified as scope.Name",
				value,
			),
		})
	}
	for _, name := range sortedMapKeys(entities) {
		ent, ok := entities[name].(map[string]any)
		if !ok {
			continue
		}
		if parent, ok := ent["subtypeOf"].(string); ok && qualifiedRefRE.MatchString(parent) {
			report(fmt.Sprintf("/entities/%s/subtypeOf", name), parent)
		}
		rels, ok := ent["relationships"].([]any)
		if !ok {
			continue
		}
		for i, r := range rels {
			rel, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if target, ok := rel["entity"].(string); ok && qualifiedRefRE.MatchString(target) {
				report(fmt.Sprintf("/entities/%s/relationships/%d/entity", name, i), target)
			}
		}
	}
	return reported, scopes
}
