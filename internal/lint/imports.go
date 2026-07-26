package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/stacklok/modelith/internal/model"
)

// FileReader reads an imported model file by path. It is the seam that keeps
// lint's file access substitutable in tests.
//
// It is not an fs.FS: fs.ValidPath rejects "..", and peer models commonly sit
// in sibling directories, so "../payments/payments.modelith.yaml" has to work.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// OSFiles reads imported models from the local filesystem.
type OSFiles struct{}

// ReadFile reads the named file from the local filesystem.
func (OSFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

var (
	// qualifiedRefRE matches a cross-model reference, "scope.Name". The scope
	// side mirrors the schema's scope pattern; the name side stays loose so a
	// reference to something that isn't a defined enum is reported as a broken
	// reference rather than silently read as a primitive type.
	qualifiedRefRE = regexp.MustCompile(`^([a-z][a-z0-9-]*)\.([A-Za-z0-9]+)$`)
	// scopeRE is the slug a bare import's filename has to yield. An explicitly
	// written scope is held to the same pattern by the schema.
	scopeRE = regexp.MustCompile(`^[a-z][a-z0-9-]+$`)
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
// its directory.
func runImports(modelPath string, m *model.Model, fr FileReader, res *Result) {
	byScope := loadImports(modelPath, m, fr, res)
	used := checkQualifiedTypes(m, byScope, res)
	// An unreferenced import is a completeness finding, alongside the unused
	// enum and the unused glossary term: vocabulary the model declares and
	// nothing uses. Sharing their category means sharing their promotion under
	// --completeness error.
	for _, scope := range sortedMapKeys(byScope) {
		if used[scope] {
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
// keyed by the scope the importing model binds them to. Every rejection is
// reported as an error: an import that cannot be resolved is a broken
// reference, not a gap.
func loadImports(modelPath string, m *model.Model, fr FileReader, res *Result) map[string]importedModel {
	byScope := map[string]importedModel{}
	dir := filepath.Dir(modelPath)
	for i, imp := range m.Imports {
		reject := func(format string, args ...any) {
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
		if filepath.IsAbs(imp.Path) || imp.Path[0] == '/' {
			reject("import %q is an absolute path — imports are relative to this model so they resolve in any checkout", imp.Path)
			continue
		}
		// A written scope is held to the pattern by the schema; a derived one the
		// schema never sees, and a filename that yields no usable slug is the
		// case the explicit form exists for.
		if imp.ScopeFromPath && !scopeRE.MatchString(imp.Scope) {
			reject("import %q binds the scope %q taken from its filename, which is not a valid slug (lowercase kebab-case) — name the scope explicitly instead: `- {scope: <slug>, path: %s}`",
				imp.Path, imp.Scope, imp.Path)
			continue
		}
		data, err := fr.ReadFile(filepath.Join(dir, imp.Path))
		if err != nil {
			reject("import %q cannot be read: %v", imp.Path, err)
			continue
		}
		im, err := model.Parse(data)
		if err != nil || im.Kind != "DomainModel" {
			reject("import %q is not a domain model — lint it on its own with `modelith lint` to see why", imp.Path)
			continue
		}
		if prev, ok := byScope[imp.Scope]; ok {
			reject("import %q binds scope %q, which import %q already binds — give one of them an explicit, different scope so %s.Name resolves unambiguously",
				imp.Path, imp.Scope, prev.path, imp.Scope)
			continue
		}
		byScope[imp.Scope] = importedModel{index: i, path: imp.Path, model: im}
	}
	return byScope
}

// checkQualifiedTypes resolves every qualified attribute type against the
// imports and returns the set of scopes that were referenced.
//
// A qualified type that does not resolve is an error, where an unqualified
// PascalCase type that names no enum is only a warning (runSemantic). The
// asymmetry is deliberate: an unqualified name may be a primitive the author
// invented, while "scope.Name" can only be a cross-model reference.
func checkQualifiedTypes(m *model.Model, byScope map[string]importedModel, res *Result) map[string]bool {
	used := map[string]bool{}
	for _, name := range m.EntityNames() {
		for i, attr := range m.Entities[name].Attributes {
			match := qualifiedRefRE.FindStringSubmatch(attr.Type)
			if match == nil {
				continue
			}
			scope, item := match[1], match[2]
			path := fmt.Sprintf("/entities/%s/attributes/%d/type", name, i)
			broken := func(format string, args ...any) {
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityError,
					Category: CategorySemantic,
					Path:     path,
					Message:  fmt.Sprintf(format, args...),
				})
			}
			imp, ok := byScope[scope]
			if !ok {
				broken("attribute type %q references the scope %q, which no import binds — add the model that defines %s to `imports:`", attr.Type, scope, item)
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
			broken("attribute type %q names no enum %q in %q — an imported model's own imports are not reachable from here", attr.Type, item, imp.path)
		}
	}
	return used
}

// reportQualifiedEntityRefs reports a cross-model reference in an entity
// position — relationship.entity or subtypeOf — and returns the instance paths
// it reported, so the schema's own finding for the same value is suppressed.
//
// Both fields carry pattern ^[A-Z][A-Za-z0-9]+$, so "payments.Card" already
// fails validation with a message about a pattern. This says what is actually
// wrong, in the spirit of the unsupported-version check. Cross-model entity
// references are deferred, not planned against: ADR-0010 records why.
func reportQualifiedEntityRefs(inst any, res *Result) map[string]bool {
	reported := map[string]bool{}
	doc, ok := inst.(map[string]any)
	if !ok {
		return reported
	}
	entities, ok := doc["entities"].(map[string]any)
	if !ok {
		return reported
	}
	report := func(path, value string) {
		reported[path] = true
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
	return reported
}
