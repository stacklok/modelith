package model

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestADR_0009_NoSelfModel guards ADR-0009: modelith's own domain is not
// modelled in a *.modelith.yaml. The vocabulary lives in the schema, these
// structs, and docs/06-schema-reference.md, two of which TestSchemaStructSync
// holds in sync; a self-model would be a fourth, unchecked copy.
//
// The allowlist is the mechanism: a new model file in the repo fails this test
// until someone adds it here, which puts the ADR in front of them.
func TestADR_0009_NoSelfModel(t *testing.T) {
	t.Parallel()

	allowed := map[string]string{
		"examples/example.modelith.yaml":                "the worked example (golden fixture)",
		"docs/05-parking-garage/garage.modelith.yaml":   "the docs example (golden fixture)",
		"docs/05-parking-garage/payments.modelith.yaml": "the docs example's imported peer model (golden fixture)",
	}

	root := repoRoot(t)
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip dot-directories: .git, and the gitignored .scratch/ and
		// .claude/worktrees/, which are full of throwaway fixtures.
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".modelith.yaml") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking repo: %v", err)
	}
	slices.Sort(found)

	for _, rel := range found {
		if _, ok := allowed[rel]; !ok {
			t.Errorf("unexpected model file %s\n"+
				"modelith does not model itself: see project-docs/adr/0009-modelith-is-not-modelled-in-modelith.md.\n"+
				"If this is a legitimate new example, add it to the allowlist in this test.", rel)
		}
	}
	for rel, why := range allowed {
		if !slices.Contains(found, rel) {
			t.Errorf("allowlisted model file %s is missing (%s); update the allowlist if it moved", rel, why)
		}
	}
}

// repoRoot walks up from the test's working directory to the directory holding
// go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod in any parent)")
		}
		dir = parent
	}
}
