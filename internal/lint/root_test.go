package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// m5Tree builds the layout the confinement cases share and returns its base,
// with symlinks already resolved so a test can predict the paths a diagnostic
// will name (a temp directory is reached through a symlink on macOS).
//
//	<base>/repo/            .git as a directory
//	<base>/repo/docs/       link -> <base>/outside/p…, dangle -> a missing file
//	<base>/repo-evil/       a sibling sharing "repo" as a string prefix
//	<base>/outside/
//	<base>/wt/              .git as a regular file, the linked-worktree shape
//	<base>/loose/{a,ab,b}/  no repository anywhere above
//	<base>/outer/           .git as a directory
//	<base>/outer/work       link -> <base>/inner/models, a directory in another repository
//	<base>/inner/           .git as a directory
func m5Tree(t *testing.T) string {
	t.Helper()

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The no-repository cases only mean anything if nothing above the temp
	// directory is a repository.
	for cur := base; ; {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			t.Skipf("temp directory %s sits inside a repository (%s), so the no-repository cases cannot be built", base, cur)
		}
		cur = parent
	}

	for _, dir := range []string{"repo/docs", "repo-evil", "outside", "wt/docs", "loose/a", "loose/ab", "loose/b", "outer", "inner/models"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// A repository marked by a directory, and one marked by the regular file a
	// linked worktree and a submodule leave behind.
	for _, dir := range []string{"repo", "outer", "inner"} {
		if err := os.Mkdir(filepath.Join(base, dir, ".git"), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "wt", ".git"), []byte("gitdir: /elsewhere/.git/worktrees/wt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"repo", "repo/docs", "repo-evil", "outside", "wt", "loose/a", "loose/ab", "loose/b", "inner/models"} {
		if err := os.WriteFile(filepath.Join(base, dir, "p.modelith.yaml"), []byte(paymentsModel), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A directory in one repository reached through a link in another, so a
	// root taken from the path as written comes from the wrong tree.
	if err := os.Symlink(filepath.Join(base, "inner", "models"), filepath.Join(base, "outer", "work")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside", "p.modelith.yaml"), filepath.Join(base, "repo/docs", "link.modelith.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside", "gone.modelith.yaml"), filepath.Join(base, "repo/docs", "dangle.modelith.yaml")); err != nil {
		t.Fatal(err)
	}
	return base
}

// TestADR_0013_ImportsConfinedToTheRepository pins the resolution boundary: an
// import may not name a file outside the repository enclosing the model, and
// outside a repository it may not leave the model's own directory. Reading a
// file is what leaks, so containment is decided before the read — the four
// outcomes a read produces are the oracle ADR-0013 closes.
func TestADR_0013_ImportsConfinedToTheRepository(t *testing.T) {
	t.Parallel()

	base := m5Tree(t)

	cases := []struct {
		name  string
		dir   string // where the model under test is written, relative to base
		entry string // the imports entry, written verbatim
		// resolved and root are relative to base. Empty resolved means the
		// import is expected to be allowed.
		resolved string
		root     string
		inRepo   bool
	}{
		{
			name:  "peer inside the repository",
			dir:   "repo/docs",
			entry: "{scope: p, path: ./p.modelith.yaml}",
		},
		{
			name:  "parent-relative inside the repository",
			dir:   "repo/docs",
			entry: "{scope: p, path: ../p.modelith.yaml}",
		},
		{
			name:  "model sitting at the repository root",
			dir:   "repo",
			entry: "{scope: p, path: ./p.modelith.yaml}",
		},
		{
			name:  "linked worktree, where .git is a file",
			dir:   "wt/docs",
			entry: "{scope: p, path: ../p.modelith.yaml}",
		},
		{
			name:     "escaping the repository root",
			dir:      "repo/docs",
			entry:    "{scope: p, path: ../../outside/p.modelith.yaml}",
			resolved: "outside/p.modelith.yaml",
			root:     "repo",
			inRepo:   true,
		},
		{
			// Judged by where the link points, not by where the link file sits.
			name:     "symlink inside the repository pointing outside",
			dir:      "repo/docs",
			entry:    "{scope: p, path: ./link.modelith.yaml}",
			resolved: "outside/p.modelith.yaml",
			root:     "repo",
			inRepo:   true,
		},
		{
			// A dangling link would otherwise pass containment and be reported
			// as unreadable, which is the oracle answer the boundary removes.
			name:     "dangling symlink pointing outside",
			dir:      "repo/docs",
			entry:    "{scope: p, path: ./dangle.modelith.yaml}",
			resolved: "outside/gone.modelith.yaml",
			root:     "repo",
			inRepo:   true,
		},
		{
			// A string prefix would read "…/repo-evil" as inside "…/repo".
			name:     "sibling whose name shares a prefix with the root",
			dir:      "repo/docs",
			entry:    "{scope: p, path: ../../repo-evil/p.modelith.yaml}",
			resolved: "repo-evil/p.modelith.yaml",
			root:     "repo",
			inRepo:   true,
		},
		{
			// The root comes from the tree the model really sits in. Climbing
			// the path as written finds <base>/outer, which holds neither the
			// model nor its peer, and refuses a file in the same directory.
			name:  "model reached through a symlinked directory",
			dir:   "outer/work",
			entry: "{scope: p, path: ./p.modelith.yaml}",
		},
		{
			// The boundary still holds from there, and names the real tree's
			// root rather than the one the link was written under.
			name:     "escaping from a symlinked directory",
			dir:      "outer/work",
			entry:    "{scope: p, path: ../../outside/p.modelith.yaml}",
			resolved: "outside/p.modelith.yaml",
			root:     "inner",
			inRepo:   true,
		},
		{
			name:  "no repository, same directory",
			dir:   "loose/a",
			entry: "{scope: p, path: ./p.modelith.yaml}",
		},
		{
			name:     "no repository, sibling directory",
			dir:      "loose/a",
			entry:    "{scope: p, path: ../b/p.modelith.yaml}",
			resolved: "loose/b/p.modelith.yaml",
			root:     "loose/a",
		},
		{
			name:     "no repository, sibling sharing a prefix",
			dir:      "loose/a",
			entry:    "{scope: p, path: ../ab/p.modelith.yaml}",
			resolved: "loose/ab/p.modelith.yaml",
			root:     "loose/a",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			modelPath := filepath.Join(base, tc.dir, fmt.Sprintf("g%d.modelith.yaml", i))
			src := importer([]string{tc.entry}, "p.PaymentMethod")
			if err := os.WriteFile(modelPath, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			res, err := Run(modelPath, []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}

			if tc.resolved == "" {
				assertFindings(t, importFindings(res.Findings), nil)
				return
			}
			assertFindings(t, importFindings(res.Findings), []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				fmt.Sprintf("resolves to %q, outside %q",
					filepath.Join(base, tc.resolved), filepath.Join(base, tc.root)),
			}})

			// The message says where the root came from; the tighter no-repository
			// root is the surprising one and has to explain itself.
			origin := "this model is in no repository, so resolution is confined to the directory holding it"
			if tc.inRepo {
				origin = "the nearest ancestor with a .git entry"
			}
			if got := res.Findings[0].Message; !strings.Contains(got, origin) {
				t.Errorf("message %q does not say where the root came from (%q)", got, origin)
			}
			// No flag names the root, so nothing may advise passing one.
			if strings.Contains(res.Findings[0].Message, "--root") {
				t.Errorf("message advises a flag that does not exist: %q", res.Findings[0].Message)
			}
		})
	}
}

// TestWithinRoot checks the containment test on its own, including the pair a
// string prefix gets wrong.
func TestWithinRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		root, candidate string
		want            bool
	}{
		{"/repo", "/repo/docs/p.modelith.yaml", true},
		{"/repo", "/repo", true},
		{"/repo", "/repo-evil/p.modelith.yaml", false},
		{"/repo", "/repository/p.modelith.yaml", false},
		{"/repo", "/etc/passwd", false},
		{"/repo", "/", false},
		{"/repo/docs", "/repo/p.modelith.yaml", false},
	}
	for _, tc := range cases {
		root, candidate := filepath.FromSlash(tc.root), filepath.FromSlash(tc.candidate)
		if got := withinRoot(root, candidate); got != tc.want {
			t.Errorf("withinRoot(%q, %q) = %v, want %v", root, candidate, got, tc.want)
		}
	}
}

// TestRealPath_SymlinkLoopTerminates checks the bound on the link-following
// filepath.EvalSymlinks refuses to do: a cycle has to return, not spin.
func TestRealPath_SymlinkLoopTerminates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	// The value is unimportant — that the call returns at all is the point.
	if got := realPath(a); got == "" {
		t.Error("realPath returned an empty path for a symlink cycle")
	}
}
