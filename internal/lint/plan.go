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

// Plan runs full lint for explicit inputs and provenance verification for their
// locally reachable imported models.
func Plan(inputs []Input, files Files) ([]FileResult, error) {
	if files == nil {
		files = OSFiles{}
	}

	results := make([]FileResult, 0, len(inputs))
	visited := make(map[string]bool, len(inputs))
	queue := make([]loadedImport, 0, len(inputs))

	for _, input := range inputs {
		path := files.Resolve(input.Path)
		if visited[path] {
			continue
		}
		visited[path] = true

		result, err := Run(input.Path, input.Source, files)
		if err != nil {
			return nil, err
		}
		results = append(results, FileResult{File: input.Path, Findings: result.Findings})

		if imported := traversableModel(input.Source); imported != nil {
			queue = append(queue, loadedImport{resolvedPath: path, source: input.Source, model: imported})
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
			if visited[loaded.resolvedPath] || (failure != nil && loaded.source == nil) {
				continue
			}
			visited[loaded.resolvedPath] = true

			res := &Result{}
			runProvenance(loaded.resolvedPath, loaded.source, res)
			sortFindings(res)
			if len(res.Findings) > 0 {
				results = append(results, FileResult{File: loaded.resolvedPath, Findings: res.Findings})
			}
			if failure == nil {
				queue = append(queue, loaded)
			}
		}
	}

	return results, nil
}

// traversableModel accepts parsed domain models that this build supports.
func traversableModel(source []byte) *model.Model {
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
