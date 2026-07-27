package deps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stacklok/modelith/internal/provenance"
)

// fakeRunner answers the gh calls Import makes from a map keyed by the API
// endpoint. A hand-written fake rather than a mock: it behaves like gh,
// answering the two endpoints it knows and failing the way gh fails on
// anything else, so a test cannot accidentally assert a response the real
// command could never produce.
type fakeRunner struct {
	content string
	sha     string
	// calls records every argv, so a test can assert what was executed.
	calls [][]string
	// fail, when set, is returned for any call whose endpoint contains it.
	fail string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	endpoint := args[len(args)-1]
	for _, a := range args {
		if strings.HasPrefix(a, "repos/") {
			endpoint = a
		}
	}
	if f.fail != "" && strings.Contains(endpoint, f.fail) {
		return nil, fmt.Errorf("gh: HTTP 404: Not Found (%s)", endpoint)
	}
	switch {
	case strings.Contains(endpoint, "/contents/"):
		return []byte(f.content), nil
	case strings.Contains(endpoint, "/commits"):
		return []byte(f.sha + "\n"), nil
	}
	return nil, fmt.Errorf("gh: unexpected endpoint %q", endpoint)
}

const upstream = `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
title: Payments
enums:
  PaymentMethod:
    values:
      - name: card
`

const blobURL = "https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml"

const sha = "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21"

func TestParseSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		ref     string
		want    Source
		wantErr string
	}{
		{
			name: "a browser blob URL",
			raw:  blobURL,
			want: Source{
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "main", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			name: "a query and anchor are not part of the address",
			raw:  blobURL + "?plain=1#L12",
			want: Source{
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "main", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			name: "an explicit ref overrides the one in the URL",
			raw:  blobURL,
			ref:  "v2.1.0",
			want: Source{
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "v2.1.0", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			// The URL alone cannot say where a slashed ref ends and the path
			// begins; naming the ref settles it.
			name: "an explicit slashed ref splits the rest correctly",
			raw:  "https://github.com/acme/billing/blob/release/v2/docs/payments.modelith.yaml",
			ref:  "release/v2",
			want: Source{
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "release/v2", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			name:    "another host asks for an issue rather than guessing",
			raw:     "https://gitlab.com/acme/billing/-/blob/main/payments.modelith.yaml",
			wantErr: "github.com/stacklok/modelith/issues",
		},
		{
			name:    "a repository URL names no file",
			raw:     "https://github.com/acme/billing",
			wantErr: "not a GitHub file URL",
		},
		{
			name:    "a tree URL is not a file URL",
			raw:     "https://github.com/acme/billing/tree/main/docs",
			wantErr: "not a GitHub file URL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSource(tc.raw, tc.ref)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("ParseSource() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func importInto(t *testing.T, dir string, r *fakeRunner, url string) (*Result, error) {
	t.Helper()
	return Import(context.Background(), Options{
		URL: url,
		Dir: dir,
		Now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run: r,
	})
}

func TestImport_StampsAVerifiableCopy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := &fakeRunner{content: upstream, sha: sha}
	res, err := importInto(t, dir, r, blobURL)
	if err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Path, filepath.Join(dir, "payments.modelith.yaml"); got != want {
		t.Errorf("wrote %s, want %s", got, want)
	}
	if res.Replaced {
		t.Error("reported replacing a file that did not exist")
	}

	h, problems := provenance.Parse(written)
	if len(problems) != 0 {
		t.Fatalf("the stamped copy has header problems: %+v", problems)
	}
	want := provenance.Header{
		Vendored: provenance.Banner,
		Fetch:    "git",
		Origin:   "https://github.com/acme/billing",
		Path:     "docs/payments.modelith.yaml",
		Ref:      "main",
		Commit:   sha,
		Imported: "2026-07-27",
		Digest:   provenance.Digest([]byte(upstream)),
	}
	if *h != want {
		t.Errorf("stamped header = %+v, want %+v", *h, want)
	}
	if ok, got := h.Verify(written); !ok {
		t.Errorf("the freshly written copy does not verify: computed %s", got)
	}

	// The editor directive stays first; the model survives byte for byte.
	if !strings.HasPrefix(string(written), "# yaml-language-server:") {
		t.Error("the editor directive is no longer the first line")
	}
	if !strings.Contains(string(written), "  PaymentMethod:\n") {
		t.Error("the model content did not survive the stamp")
	}
}

// TestImport_CallsGhWithTheExpectedEndpoints asserts the whole argv of both
// calls, not that they merely contain something. The endpoints are the contract
// with gh — a ref silently dropped from the content fetch would return whatever
// the default branch holds, and the copy would be stamped with a ref it is not
// actually at.
//
// Nothing here goes through a shell; that is a property of ExecRunner passing
// an argv array to os/exec, which no assertion about argument text could show.
func TestImport_CallsGhWithTheExpectedEndpoints(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{content: upstream, sha: sha}
	if _, err := importInto(t, t.TempDir(), r, blobURL); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"gh", "api", "-H", "Accept: application/vnd.github.raw",
			"repos/acme/billing/contents/docs/payments.modelith.yaml?ref=main"},
		{"gh", "api",
			"repos/acme/billing/commits?path=docs%2Fpayments.modelith.yaml&sha=main&per_page=1",
			"--jq", ".[0].sha"},
	}
	if len(r.calls) != len(want) {
		t.Fatalf("want %d gh calls, got %d: %+v", len(want), len(r.calls), r.calls)
	}
	for i, call := range r.calls {
		if strings.Join(call, "\x00") != strings.Join(want[i], "\x00") {
			t.Errorf("call %d:\n got %q\nwant %q", i, call, want[i])
		}
	}
}

// TestImport_EscapesAPathThatNeedsIt pins that a path segment reaches the API
// escaped rather than as a second query parameter or a truncated path.
func TestImport_EscapesAPathThatNeedsIt(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{content: upstream, sha: sha}
	url := "https://github.com/acme/billing/blob/main/docs/a b&c/payments.modelith.yaml"
	if _, err := importInto(t, t.TempDir(), r, url); err != nil {
		t.Fatal(err)
	}
	const wantContents = "repos/acme/billing/contents/docs/a%20b&c/payments.modelith.yaml?ref=main"
	if got := r.calls[0][len(r.calls[0])-1]; got != wantContents {
		t.Errorf("content endpoint = %q, want %q", got, wantContents)
	}
}

// TestADR_0015_ImportFetchesOneFileNotATree pins that vendoring does not
// recurse. The fetched model's own imports are reported to the user and
// nothing more: no extra call goes out, so no file arrives that the user did
// not ask for, at a scope this repository never bound.
func TestADR_0015_ImportFetchesOneFileNotATree(t *testing.T) {
	t.Parallel()

	chained := upstream + "imports:\n  - ./ledger.modelith.yaml\n  - ./tax.modelith.yaml\n"
	r := &fakeRunner{content: chained, sha: sha}
	res, err := importInto(t, t.TempDir(), r, blobURL)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"./ledger.modelith.yaml", "./tax.modelith.yaml"}
	if strings.Join(res.TheirImports, ",") != strings.Join(want, ",") {
		t.Errorf("TheirImports = %v, want %v", res.TheirImports, want)
	}
	// Two calls and no more: the imports were reported, not followed.
	if len(r.calls) != 2 {
		t.Errorf("want two gh calls, got %d: %+v", len(r.calls), r.calls)
	}
}

func TestImport_Rejections(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		runner  *fakeRunner
		url     string
		wantErr string
	}{
		{
			name:    "a file that is already a vendored copy",
			runner:  &fakeRunner{content: "# modelith-origin: https://github.com/other/repo\n" + upstream, sha: sha},
			url:     blobURL,
			wantErr: "itself a vendored copy",
		},
		{
			name:    "a file that is not a domain model",
			runner:  &fakeRunner{content: "kind: SomethingElse\nversion: v1\n", sha: sha},
			url:     blobURL,
			wantErr: "not a domain model",
		},
		{
			name:    "a path with no commits at that ref",
			runner:  &fakeRunner{content: upstream, sha: "null"},
			url:     blobURL,
			wantErr: "no commit touching",
		},
		{
			name:    "gh refusing the fetch",
			runner:  &fakeRunner{content: upstream, sha: sha, fail: "/contents/"},
			url:     blobURL,
			wantErr: "HTTP 404",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := importInto(t, dir, tc.runner, tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Errorf("a rejected import left %d file(s) behind", len(entries))
			}
		})
	}
}

func TestImport_ReplacesAnEarlierCopy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := &fakeRunner{content: upstream, sha: sha}
	if _, err := importInto(t, dir, r, blobURL); err != nil {
		t.Fatal(err)
	}
	moved := &fakeRunner{content: upstream + "  # a later revision\n", sha: "9" + sha[1:]}
	res, err := importInto(t, dir, moved, blobURL)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replaced {
		t.Error("did not report replacing the earlier copy")
	}
	written, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := provenance.Parse(written)
	if h.Commit != moved.sha {
		t.Errorf("commit is %s, want the newer %s", h.Commit, moved.sha)
	}
	if ok, _ := h.Verify(written); !ok {
		t.Error("the replaced copy does not verify against its own digest")
	}
	if strings.Count(string(written), provenance.LinePrefix+"origin:") != 1 {
		t.Error("the replaced copy carries more than one header")
	}
}
