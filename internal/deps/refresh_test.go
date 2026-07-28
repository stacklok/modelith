package deps

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// laterSHA is the commit an origin has moved to. It differs from sha in every
// position that matters, so a test cannot pass by comparing a prefix.
const laterSHA = "a91b0c37e5d248fa6c0913be4b7f25d81a3c6e90"

// updatedAt is the day an update runs, a day after importedAt, so a header's
// imported: line proves which of the two wrote it.
var (
	importedAt = time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local)
	updatedAt  = time.Date(2026, 7, 28, 9, 0, 0, 0, time.Local)
)

// vendored writes a copy of content into a fresh temp dir exactly as deps
// import would, and hands back its path with the runner that served it. The
// fixture is built by the real Import so a test reasons about a copy that looks
// like one a user has, rather than one hand-assembled to suit the assertion.
//
// The runner's call log is cleared before returning, so a test counting calls
// counts only its own.
func vendored(t *testing.T, content string) (string, *fakeRunner) {
	t.Helper()
	r := &fakeRunner{content: content, sha: sha}
	res, err := Import(context.Background(), Options{
		URL: blobURL, Dir: t.TempDir(), Now: importedAt, Run: r,
	})
	if err != nil {
		t.Fatalf("building the fixture: %v", err)
	}
	r.calls = nil
	return res.Path, r
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func check(t *testing.T, r Runner, paths ...string) []Report {
	t.Helper()
	reports, err := Check(context.Background(), CheckOptions{Paths: paths, Run: r})
	if err != nil {
		t.Fatalf("check aborted: %v", err)
	}
	return reports
}

func update(t *testing.T, r Runner, ref string, paths ...string) []Report {
	t.Helper()
	reports, err := Update(context.Background(), UpdateOptions{
		Paths: paths, Ref: ref, Now: updatedAt, Run: r,
	})
	if err != nil {
		t.Fatalf("update aborted: %v", err)
	}
	return reports
}

// moved is upstream with a real change to its content: an enum gains a value.
const moved = `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
title: Payments
enums:
  PaymentMethod:
    values:
      - name: card
      - name: bank-transfer
`

func TestCheck_ReportsWhetherTheOriginMoved(t *testing.T) {
	t.Parallel()

	t.Run("an unmoved origin is up to date", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		rep := check(t, r, path)[0]
		if rep.Err != nil {
			t.Fatalf("unexpected error: %v", rep.Err)
		}
		if rep.Stale() {
			t.Error("reported stale against an origin serving the same bytes")
		}
		if rep.State.Ref != "main" {
			t.Errorf("checked against ref %q, want the header's %q", rep.State.Ref, "main")
		}
	})

	t.Run("a moved origin is stale and names the new commit", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		r.content, r.sha = moved, laterSHA
		rep := check(t, r, path)[0]
		if !rep.Stale() {
			t.Fatal("reported up to date against an origin serving different bytes")
		}
		if rep.Commit != laterSHA {
			t.Errorf("Commit = %q, want the origin's %q", rep.Commit, laterSHA)
		}
	})

	t.Run("check writes nothing", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		before := readFile(t, path)
		r.content, r.sha = moved, laterSHA
		check(t, r, path)
		if after := readFile(t, path); after != before {
			t.Error("check rewrote the file it was only asked to report on")
		}
	})
}

// TestADR_0016_StalenessIsContentNotCommit pins the decision that makes check
// worth running: the verdict is what the origin serves, not which commit last
// touched the path. A commit that changes no bytes — a merge, a rename, a
// whitespace touch — must not report as a change, and a change must report even
// if the commit somehow did not move.
//
// Neither half computes a digest. Both read the verdict off content the test
// controls directly, so an implementation that hashed the wrong bytes could not
// satisfy them by hashing them the same wrong way.
func TestADR_0016_StalenessIsContentNotCommit(t *testing.T) {
	t.Parallel()

	t.Run("a new commit over identical bytes is not stale", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		r.sha = laterSHA // the commit moved; the file did not
		rep := check(t, r, path)[0]
		if rep.Stale() {
			t.Error("a commit that changed no bytes reported as stale")
		}
		// The commit is reporting, not verdict, so nothing asked for it.
		if rep.Commit != "" {
			t.Errorf("resolved a commit for an unmoved origin: %q", rep.Commit)
		}
	})

	t.Run("changed bytes under the same commit are stale", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		r.content = moved // the file changed; the commit did not
		rep := check(t, r, path)[0]
		if !rep.Stale() {
			t.Error("changed bytes under an unchanged commit reported as up to date")
		}
	})
}

// TestCheck_CostsOneCallPerCleanCopy pins the lazy commit fetch. Resolving the
// SHA for every copy would double the API cost of the command people are meant
// to run habitually, and nothing else would fail if it were resolved eagerly.
func TestCheck_CostsOneCallPerCleanCopy(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	check(t, r, path)
	if len(r.calls) != 1 {
		t.Errorf("a clean check made %d gh calls, want 1: %+v", len(r.calls), r.calls)
	}

	r.calls, r.content, r.sha = nil, moved, laterSHA
	check(t, r, path)
	if len(r.calls) != 2 {
		t.Errorf("a stale check made %d gh calls, want 2: %+v", len(r.calls), r.calls)
	}
}

func TestCheck_SkipsAFileWithNoProvenanceHeader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	own := dir + "/ours.modelith.yaml"
	if err := os.WriteFile(own, []byte(upstream), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{content: upstream, sha: sha}
	rep := check(t, r, own)[0]
	if !rep.Skipped {
		t.Error("a model this repository owns was not skipped")
	}
	if len(r.calls) != 0 {
		t.Errorf("a skipped file still cost %d gh calls: %+v", len(r.calls), r.calls)
	}
}

// TestSurvey_ContinuesPastAFailure pins that one unreachable copy does not hide
// the state of the files after it. Aborting on the third of eight turns one run
// into five sequential ones.
func TestSurvey_ContinuesPastAFailure(t *testing.T) {
	t.Parallel()

	good, r := vendored(t, upstream)
	missing := t.TempDir() + "/gone.modelith.yaml"

	reports := check(t, r, missing, good)
	if len(reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(reports))
	}
	if reports[0].Err == nil {
		t.Error("an unreadable file did not report an error")
	}
	if reports[1].Err != nil || reports[1].State == nil {
		t.Errorf("the file after the failure was not checked: %+v", reports[1])
	}
}

// TestSurvey_StopsWhenTheToolIsUnusable pins the single exception to
// continue-on-failure. gh being absent or unauthenticated fails every file
// identically, so repeating the paragraph once per copy is noise.
func TestSurvey_StopsWhenTheToolIsUnusable(t *testing.T) {
	t.Parallel()

	a, _ := vendored(t, upstream)
	b, _ := vendored(t, upstream)

	r := &unusableRunner{}
	reports, err := Check(context.Background(), CheckOptions{Paths: []string{a, b}, Run: r})
	if !errors.Is(err, ErrToolUnavailable) {
		t.Fatalf("Check returned %v, want an ErrToolUnavailable", err)
	}
	// The file that hit it gets no Report. It was abandoned mid-flight, so it
	// has neither a verdict nor a per-file failure, and a Report carrying
	// neither is a hole every consumer has to remember to check — one of them
	// did not, and dereferenced its nil State.
	if len(reports) != 0 {
		t.Errorf("got %d reports, want none — the run never judged a file: %+v", len(reports), reports)
	}
	if r.calls != 1 {
		t.Errorf("the run made %d calls after gh proved unusable, want 1", r.calls)
	}
}

// unusableRunner is gh being absent: every call fails the same way.
type unusableRunner struct{ calls int }

func (u *unusableRunner) Run(context.Context, string, ...string) ([]byte, error) {
	u.calls++
	return nil, unusable{errors.New("gh is not installed — modelith delegates fetching to it")}
}

// TestUnauthenticatedMatchesGhsOwnText pins the needles against the messages gh
// actually emits, quoted from its binary. gh reports this on stderr and exits 1,
// so there is no typed error to unwrap and nothing else to key on.
func TestUnauthenticatedMatchesGhsOwnText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"logged out entirely", "To get started with GitHub CLI, please run:  gh auth login", true},
		{"a workflow with no token", "gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN environment variable.", true},
		{"automation with no token", "gh: To use GitHub CLI in automation, set the GH_TOKEN environment variable.", true},
		{"a token that is refused", "gh: Bad credentials (HTTP 401)", true},
		{"a file that is not there", "gh: HTTP 404: Not Found (https://api.github.com/repos/a/b/contents/c)", false},
		{"a repository that is private", "gh: HTTP 403: Forbidden", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := unauthenticated(tc.msg); got != tc.want {
				t.Errorf("unauthenticated(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

// TestADR_0016_ACurrentCopyIsNotRewritten pins the property that makes update
// safe to run habitually over a glob: it produces a diff only where something
// changed. It also pins what imported: means — a date that moved here would be
// dating the run rather than the version.
//
// The assertion is on the file's bytes, not on an absence of errors: a rewrite
// that happened to produce the same header would still be a rewrite this test
// must not accept, and comparing bytes is the only way to see one.
func TestADR_0016_ACurrentCopyIsNotRewritten(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	before := readFile(t, path)

	rep := update(t, r, "", path)[0]
	if rep.Written {
		t.Error("update reported writing a copy whose origin had not moved")
	}
	if after := readFile(t, path); after != before {
		t.Errorf("update rewrote a current copy:\n before %q\n  after %q", before, after)
	}
	if !strings.Contains(before, "# modelith-imported: 2026-07-27") {
		t.Errorf("the fixture's imported date is not the import's:\n%s", before)
	}
}

func TestUpdate_RefreshesAMovedOrigin(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	r.content, r.sha = moved, laterSHA

	rep := update(t, r, "", path)[0]
	if !rep.Written || rep.Restored {
		t.Fatalf("want a refresh, got Written=%v Restored=%v (%v)", rep.Written, rep.Restored, rep.Err)
	}
	after := readFile(t, path)
	for _, want := range []string{
		"# modelith-commit: " + laterSHA,
		"# modelith-imported: 2026-07-28",
		"- name: bank-transfer",
	} {
		if !strings.Contains(after, want) {
			t.Errorf("the refreshed copy does not contain %q:\n%s", want, after)
		}
	}
	// The refreshed copy must verify against the digest it now records, or the
	// next lint reports the update as tampering.
	if reports := check(t, r, path); reports[0].Stale() {
		t.Error("the copy is still stale immediately after an update")
	}
	if reports := check(t, r, path); reports[0].State.Edited() {
		t.Error("the refreshed copy does not verify against its own new digest")
	}
}

// TestUpdate_RestoresAnEditedCopy pins the case the write condition folds in
// rather than special-cases: a copy that drifted from the version it claims is
// not holding what its origin serves, so it is not current, so update writes.
// Restoring reproduces the original import's bytes exactly — no commit is
// fetched, because nothing moved, and imported: does not move, because no new
// version arrived.
func TestUpdate_RestoresAnEditedCopy(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	pristine := readFile(t, path)

	edited := strings.Replace(pristine, "title: Payments", "title: Payments (ours)", 1)
	if edited == pristine {
		t.Fatal("the test did not edit the copy")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	r.calls = nil
	rep := update(t, r, "", path)[0]
	if !rep.Written || !rep.Restored {
		t.Fatalf("want a restore, got Written=%v Restored=%v (%v)", rep.Written, rep.Restored, rep.Err)
	}
	if after := readFile(t, path); after != pristine {
		t.Errorf("the restore did not reproduce the imported bytes:\n want %q\n  got %q", pristine, after)
	}
	if len(r.calls) != 1 {
		t.Errorf("a restore made %d gh calls, want 1 — the origin did not move: %+v", len(r.calls), r.calls)
	}
}

func TestUpdate_Repin(t *testing.T) {
	t.Parallel()

	t.Run("more than one file is an error", func(t *testing.T) {
		t.Parallel()
		a, r := vendored(t, upstream)
		b, _ := vendored(t, upstream)
		_, err := Update(context.Background(), UpdateOptions{
			Paths: []string{a, b}, Ref: "v2.2.0", Now: updatedAt, Run: r,
		})
		if err == nil || !strings.Contains(err.Error(), "--ref re-pins one copy") {
			t.Fatalf("want a refusal naming the flag, got %v", err)
		}
	})

	// A tag can point at a commit whose file content is byte-identical to the
	// previous tag's. The header still has to move, because the ref it records
	// is now wrong — and a header that lies about its own pin is worse than a
	// stale one, because nothing later can detect it.
	t.Run("identical content still rewrites the header", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		r.sha = laterSHA

		rep := update(t, r, "v2.2.0", path)[0]
		if !rep.Written || rep.Restored {
			t.Fatalf("want a refresh, got Written=%v Restored=%v (%v)", rep.Written, rep.Restored, rep.Err)
		}
		after := readFile(t, path)
		for _, want := range []string{
			"# modelith-ref: v2.2.0",
			"# modelith-commit: " + laterSHA,
			"# modelith-imported: 2026-07-28",
		} {
			if !strings.Contains(after, want) {
				t.Errorf("the re-pinned copy does not contain %q:\n%s", want, after)
			}
		}
	})

	t.Run("the new ref is what gets fetched", func(t *testing.T) {
		t.Parallel()
		path, r := vendored(t, upstream)
		r.sha = laterSHA
		update(t, r, "v2.2.0", path)
		if got := strings.Join(r.calls[0], " "); !strings.Contains(got, "?ref=v2.2.0") {
			t.Errorf("the content fetch did not use the new ref: %s", got)
		}
	})
}

// TestUpdate_ReportsImportsItDidNotFollow pins that a refresh bringing new
// imports says so. Vendoring is not transitive, so an import that arrives in an
// updated copy resolves to nothing until the user vendors it directly, and the
// update is the only moment anything can tell them.
func TestUpdate_ReportsImportsItDidNotFollow(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream+"imports:\n  - ./ledger.modelith.yaml\n")
	r.content = moved + "imports:\n  - ./ledger.modelith.yaml\n  - ./tax.modelith.yaml\n"
	r.sha = laterSHA

	rep := update(t, r, "", path)[0]
	if rep.Err != nil {
		t.Fatal(rep.Err)
	}
	// Only the new one: the copy already declared ledger at import time, and
	// re-warning about it on every update trains the reader to skip the note.
	if want := []string{"./tax.modelith.yaml"}; strings.Join(rep.NewImports, ",") != strings.Join(want, ",") {
		t.Errorf("NewImports = %v, want %v", rep.NewImports, want)
	}
	if len(r.calls) != 2 {
		t.Errorf("an update followed %d calls, want 2 — imports are reported, not fetched: %+v", len(r.calls), r.calls)
	}
}

// TestVisit_RefusesAnUnusableHeader pins ADR-0015's rule applied here:
// classification is generous, exemption is strict. A file with a broken header
// is still a copy, so it is not skipped — but every comparison this package
// makes rests on that header, so it earns no verdict and the user is sent to
// the tool that explains header defects.
func TestVisit_RefusesAnUnusableHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mangle  func(string) string
		wantErr string
	}{
		{
			name:    "a malformed digest",
			mangle:  func(s string) string { return strings.Replace(s, "# modelith-digest: sha256:", "# modelith-digest: ", 1) },
			wantErr: "sha256:<64 hex digits>",
		},
		{
			name:    "an unknown key",
			mangle:  func(s string) string { return "# modelith-source: elsewhere\n" + s },
			wantErr: "unknown provenance key",
		},
		{
			name:    "a missing origin",
			mangle:  func(s string) string { return strings.Replace(s, "# modelith-origin: https://github.com/acme/billing\n", "", 1) },
			wantErr: `missing "# modelith-origin"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, r := vendored(t, upstream)
			mangled := tc.mangle(readFile(t, path))
			if err := os.WriteFile(path, []byte(mangled), 0o644); err != nil {
				t.Fatal(err)
			}
			rep := check(t, r, path)[0]
			if rep.Skipped {
				t.Fatal("a copy with a broken header was skipped rather than reported")
			}
			if rep.Err == nil {
				t.Fatal("a copy with a broken header got a verdict")
			}
			for _, want := range []string{tc.wantErr, "modelith lint"} {
				if !strings.Contains(rep.Err.Error(), want) {
					t.Errorf("error does not contain %q: %v", want, rep.Err)
				}
			}
			if len(r.calls) != 0 {
				t.Errorf("a copy with a broken header was still fetched: %+v", r.calls)
			}
		})
	}
}

// TestVisit_RefusesWhatTheOriginBecame pins that the two refusals import makes
// on a first fetch are made again on a refetch. The file at an address is not
// the file that was there last time, and writing either of these over a good
// copy would be worse than leaving it stale.
func TestVisit_RefusesWhatTheOriginBecame(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		upstream string
		wantErr  string
	}{
		{
			name:     "the origin is now itself a copy",
			upstream: "# modelith-vendored: " + "DO NOT EDIT\n" + moved,
			wantErr:  "somebody else's copy",
		},
		{
			name:     "the origin no longer parses",
			upstream: "kind: DomainModel\nentities: [oops\n",
			wantErr:  "no longer parses as a domain model",
		},
		{
			name:     "the origin is no longer a domain model",
			upstream: "kind: SomethingElse\nversion: v1\n",
			wantErr:  "no longer a domain model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, r := vendored(t, upstream)
			before := readFile(t, path)
			r.content, r.sha = tc.upstream, laterSHA

			rep := update(t, r, "", path)[0]
			if rep.Err == nil || !strings.Contains(rep.Err.Error(), tc.wantErr) {
				t.Fatalf("want an error containing %q, got %v", tc.wantErr, rep.Err)
			}
			if after := readFile(t, path); after != before {
				t.Error("the copy was overwritten with content the fetch should have refused")
			}
		})
	}
}

// TestMovedHint offers the explanation a refetch needs and a first fetch does
// not: the address came out of a header written some time ago, and a model can
// move or be deleted without anything here noticing. It is owed only for a 404 —
// a rejected credential says nothing about where the file lives.
func TestMovedHint(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	r.fail = "/contents/"

	rep := check(t, r, path)[0]
	if rep.Err == nil {
		t.Fatal("a failed fetch reported no error")
	}
	for _, want := range []string{"the model moved", "it was deleted", "docs/payments.modelith.yaml"} {
		if !strings.Contains(rep.Err.Error(), want) {
			t.Errorf("the 404 error does not contain %q: %v", want, rep.Err)
		}
	}
}

// TestSourceFromHeader pins that a header's parts go back through the one
// parser rather than being pasted into an endpoint here. A hand-written header
// is untrusted input in exactly the way a typed URL is, and the guarantees
// escapePath depends on live in ParseSource.
func TestSourceFromHeader(t *testing.T) {
	t.Parallel()

	path, r := vendored(t, upstream)
	traversal := strings.Replace(readFile(t, path),
		"# modelith-path: docs/payments.modelith.yaml",
		"# modelith-path: docs/../../etc/passwd", 1)
	if err := os.WriteFile(path, []byte(traversal), 0o644); err != nil {
		t.Fatal(err)
	}

	rep := check(t, r, path)[0]
	if rep.Err == nil {
		t.Fatal("a header with a traversal path was fetched")
	}
	if !strings.Contains(rep.Err.Error(), `".." path segment`) {
		t.Errorf("the error does not name the traversal: %v", rep.Err)
	}
	if len(r.calls) != 0 {
		t.Errorf("a traversal path still reached gh: %+v", r.calls)
	}
}
