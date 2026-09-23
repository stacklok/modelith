package lint

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/schema"
)

// Input is one explicitly requested model and the bytes read for it.
type Input struct {
	Path   string
	Source []byte
}

// FileResult is the output for one model. Explicit inputs retain the path the
// caller supplied; discovered imports use their resolved paths.
type FileResult struct {
	File     string    `json:"file"`
	Findings []Finding `json:"findings"`
}

// Plan runs full lint for every explicit input and provenance verification for
// their locally reachable imported models. Explicit inputs retain both their
// argument order and multiplicity; resolved paths deduplicate only discovery.
func Plan(inputs []Input, files Files) ([]FileResult, error) {
	if files == nil {
		files = OSFiles{}
	}

	results := make([]FileResult, 0, len(inputs))
	explicit := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		explicit[files.Resolve(input.Path)] = true
	}
	discovered := make(map[string]bool)
	queue := make([]loadedImport, 0, len(inputs))

	for _, input := range inputs {
		result, err := Run(input.Path, input.Source, files)
		if err != nil {
			return nil, err
		}
		results = append(results, FileResult{File: input.Path, Findings: result.Findings})

		// A structurally invalid root has already received its full result above,
		// but its imports cannot safely seed the integrity-only crawl.
		if imported := traversableModel(input.Source); imported != nil {
			queue = append(queue, loadedImport{
				resolvedPath: files.Resolve(input.Path),
				source:       input.Source,
				model:        imported,
			})
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		root, inRepo := files.ResolutionRoot(current.resolvedPath)
		for _, imp := range current.model.Imports {
			if !traversableImport(imp.Path) {
				continue
			}
			loaded, failure := loadImport(current.resolvedPath, root, inRepo, imp.Path, files)
			if explicit[loaded.resolvedPath] || discovered[loaded.resolvedPath] || (failure != nil && loaded.source == nil) {
				continue
			}
			discovered[loaded.resolvedPath] = true

			res := &Result{}
			runProvenance(loaded.resolvedPath, loaded.source, res)
			sortFindings(res)
			if len(res.Findings) > 0 {
				results = append(results, FileResult{File: loaded.resolvedPath, Findings: res.Findings})
			}
			if failure == nil {
				// A broken nested edge stays silent, but a readable, structurally
				// valid model continues the integrity-only crawl.
				if imported := traversableModel(loaded.source); imported != nil {
					loaded.model = imported
					queue = append(queue, loaded)
				}
			}
		}
	}

	return results, nil
}

// traversableModel accepts structurally valid domain models that this build supports.
func traversableModel(source []byte) *model.Model {
	if len(Structural(source)) > 0 {
		return nil
	}
	m, err := model.Parse(source)
	if err != nil || m.Kind != "DomainModel" || !schema.Supported(m.Version) {
		return nil
	}
	return m
}

func traversableImport(path string) bool {
	return path != "" &&
		!filepath.IsAbs(path) &&
		!strings.HasPrefix(path, "/") &&
		strings.IndexFunc(path, unicode.IsControl) < 0
}
