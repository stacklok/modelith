package deps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stacklok/modelith/internal/provenance"
)

// fakeRunner answers the gh/az calls Import makes. It behaves like gh or az,
// answering the endpoints it knows and failing the way they fail on anything
// else, so a test cannot accidentally assert a response the real command could
// never produce.
type fakeRunner struct {
	content string
	sha     string
	// calls records every argv, so a test can assert what was executed.
	calls [][]string
	// fail, when set, is returned for any call whose endpoint contains it.
	fail string
	// ado sets whether to answer as az rest (ADO) instead of gh api.
	ado bool
	// unusable, when set, makes calls to matching endpoints fail because gh itself
	// cannot be used.
	unusable string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))

	if f.ado {
		return f.runAz(args)
	}
	return f.runGh(args)
}

func (f *fakeRunner) runGh(args []string) ([]byte, error) {
	endpoint := args[len(args)-1]
	for _, a := range args {
		if strings.HasPrefix(a, "repos/") {
			endpoint = a
		}
	}
	if f.unusable != "" && strings.Contains(endpoint, f.unusable) {
		return nil, unusable{fmt.Errorf("gh is not installed — modelith delegates fetching to it")}
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

func (f *fakeRunner) runAz(args []string) ([]byte, error) {
	var uri, resource string
	for i, a := range args {
		if a == "--uri" && i+1 < len(args) {
			uri = args[i+1]
		}
		if a == "--resource" && i+1 < len(args) {
			resource = args[i+1]
		}
	}
	// The ADO resource ID must be present.
	if resource != "499b84ac-1321-427f-aa17-267ca6975798" {
		return nil, fmt.Errorf("az: expected --resource 499b84ac-1321-427f-aa17-267ca6975798, got %q", resource)
	}
	if f.fail != "" && strings.Contains(uri, f.fail) {
		return nil, fmt.Errorf("az: HTTP 404: Not Found (%s)", uri)
	}
	switch {
	case strings.Contains(uri, "/items"):
		// Validate that a known version prefix produces the right versionType.
		// GB→branch, GT→tag, GC→commit. When the prefix is absent or unknown,
		// versionType is omitted (the API auto-detects).
		if strings.Contains(uri, "versionType") {
			if !strings.Contains(uri, "versionType=branch") &&
				!strings.Contains(uri, "versionType=tag") &&
				!strings.Contains(uri, "versionType=commit") {
				return nil, fmt.Errorf("az: unexpected versionType in %q", uri)
			}
		}
		// The real fetch writes the body to --output-file (az rest appends a
		// newline when printing a body to stdout, which would drift the
		// vendored copy — see fetchContentADO). The fake must mirror that
		// contract, so a test cannot pass while asserting an argv the real
		// command would not produce.
		var outPath string
		for i, a := range args {
			if a == "--output-file" && i+1 < len(args) {
				outPath = args[i+1]
			}
		}
		if outPath == "" {
			return nil, fmt.Errorf("az: content fetch must use --output-file, got %q", args)
		}
		// The Items API returns the file's bytes only with download=true;
		// without it the body is a JSON GitItem, which the model parser then
		// rejects. Mirroring that here turns the omission into a test failure
		// rather than a surprise against a real endpoint.
		body := []byte(f.content)
		if !strings.Contains(uri, "download=true") {
			body = []byte(`{"objectId":"` + f.sha + `","gitObjectType":"blob","path":"/payments.modelith.yaml"}`)
		}
		if err := os.WriteFile(outPath, body, 0o644); err != nil {
			return nil, err
		}
		return body, nil
	case strings.Contains(uri, "/commits"):
		// The commit endpoint must not shell-escape $top.
		if strings.Contains(uri, "\\$top") {
			return nil, fmt.Errorf("az: $top is shell-escaped, but ExecRunner uses argv (no shell)")
		}
		return []byte(f.sha + "\n"), nil
	}
	return nil, fmt.Errorf("az: unexpected uri %q", uri)
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
				Host:   HostGitHub,
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "main", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			name: "a query and anchor are not part of the address",
			raw:  blobURL + "?plain=1#L12",
			want: Source{
				Host:   HostGitHub,
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "main", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			name: "an explicit ref overrides the one in the URL",
			raw:  blobURL,
			ref:  "v2.1.0",
			want: Source{
				Host:   HostGitHub,
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
				Host:   HostGitHub,
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "release/v2", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			// A host is case-insensitive. Rejecting this one produced the
			// self-contradictory "only from github.com, and … is on GitHub.com".
			name: "the host is matched without regard to case or a www prefix",
			raw:  "https://WWW.GitHub.com/acme/billing/blob/main/docs/payments.modelith.yaml",
			want: Source{
				Host:   HostGitHub,
				Origin: "https://github.com/acme/billing", Owner: "acme", Repo: "billing",
				Ref: "main", Path: "docs/payments.modelith.yaml",
			},
		},
		{
			// url.Parse does not remove dot segments, and escapePath cannot
			// neutralise a segment that *is* a traversal — it would reach the
			// API endpoint intact and leave the contents namespace.
			name:    "a traversal segment is not an address github.com serves",
			raw:     "https://github.com/acme/billing/blob/main/../../../user/repos",
			wantErr: `has a ".." path segment`,
		},
		{
			name:    "an empty segment is rejected too",
			raw:     "https://github.com/acme/billing/blob/main//payments.modelith.yaml",
			wantErr: `has a "" path segment`,
		},
		{
			name:    "another host asks for an issue rather than guessing",
			raw:     "https://gitlab.com/acme/billing/-/blob/main/payments.modelith.yaml",
			wantErr: "github.com/stacklok/modelith/issues",
		},
		{
			// The Raw button hands this one back, so it is easy to paste. Sending
			// the reader off to ask for another host to be supported is advice
			// about the wrong problem: the model is already where modelith looks.
			name:    "a raw URL is pointed at the blob view, not at an issue",
			raw:     "https://raw.githubusercontent.com/acme/billing/main/docs/payments.modelith.yaml",
			wantErr: "the same address with /blob/ in it",
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
			wantErr: "reads it as somebody else's copy",
		},
		{
			name:    "a file that is not a domain model",
			runner:  &fakeRunner{content: "kind: SomethingElse\nversion: v1\n", sha: sha},
			url:     blobURL,
			wantErr: `it declares kind "SomethingElse"`,
		},
		{
			name:    "a file with no kind at all",
			runner:  &fakeRunner{content: "title: Payments\n", sha: sha},
			url:     blobURL,
			wantErr: "it declares no kind",
		},
		{
			// A model authored by a newer modelith parses strictly and fails.
			// Collapsing that into "is not a domain model" sent the user to
			// check the URL, which is not what went wrong.
			name:    "a model this build cannot read says what the parser said",
			runner:  &fakeRunner{content: upstream + "futureField: yes\n", sha: sha},
			url:     blobURL,
			wantErr: "futureField",
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
		{
			// The likeliest reason a fetch 404s on a URL that parsed is that
			// the ref has a slash in it and the path was split at the first
			// segment. Saying so is the difference between a dead end and a fix.
			name:    "a refused fetch says how the URL was split",
			runner:  &fakeRunner{content: upstream, sha: sha, fail: "/contents/"},
			url:     blobURL,
			wantErr: `read "main" as the ref and "docs/payments.modelith.yaml" as the path`,
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

// TestImport_RefusesToOverwriteWhatItDidNotWrite pins the guard on the
// destination. The filename comes from the origin, so it can land on a file this
// repository already has — and overwriting a model this repository wrote loses
// work no re-fetch can recover, which the earlier os.Stat-only check did
// silently, reporting it as an ordinary "replaced".
func TestImport_RefusesToOverwriteWhatItDidNotWrite(t *testing.T) {
	t.Parallel()

	const own = "kind: DomainModel\nversion: v1\ntitle: Ours\n"

	cases := []struct {
		name     string
		existing string
		wantErr  string
	}{
		{
			name:     "a model this repository owns",
			existing: own,
			wantErr:  "carries no provenance header",
		},
		{
			name: "a copy of a different model with the same filename",
			existing: "# modelith-vendored: " + provenance.Banner + "\n" +
				"# modelith-fetch: git\n" +
				"# modelith-origin: https://github.com/other/repo\n" +
				"# modelith-path: docs/payments.modelith.yaml\n" +
				"# modelith-ref: main\n# modelith-commit: abc\n" +
				"# modelith-imported: 2026-07-27\n" +
				"# modelith-digest: " + provenance.Digest([]byte(own)) + "\n" + own,
			wantErr: "is a vendored copy of https://github.com/other/repo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			target := filepath.Join(dir, "payments.modelith.yaml")
			if err := os.WriteFile(target, []byte(tc.existing), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha}, blobURL)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != tc.existing {
				t.Errorf("the refused import wrote over the file anyway:\n%s", got)
			}
		})
	}
}

func TestImport_RefusesUnknownReservedPrefixAtTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "payments.modelith.yaml")
	existing := "# modelith-note: locally owned\n" + strings.Replace(upstream, "title: Payments", "title: Ours", 1)
	if err := os.WriteFile(target, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha}, blobURL)
	if err == nil {
		t.Fatal("an unknown reserved-prefix comment allowed import to overwrite a local model")
	}
	for _, want := range []string{
		"cannot be identified as a copy of the source",
		"Import into a different directory",
		"deliberately move or delete",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not offer %q: %v", want, err)
		}
	}

	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != existing {
		t.Errorf("the refused import wrote over the file anyway:\n%s", got)
	}
}

// TestSplitHint pins that the ref/path hint is offered for the failure it
// explains and no other. It was appended to every fetch error, so "gh is not
// installed" arrived with a paragraph about ref splitting attached — advice
// about a URL that was never in question.
func TestSplitHint(t *testing.T) {
	t.Parallel()

	src := Source{Host: HostGitHub, Ref: "main", Path: "docs/payments.modelith.yaml"}
	cases := []struct {
		name string
		src  Source
		err  error
		want bool
	}{
		{"a not-found", src, fmt.Errorf("gh: HTTP 404: Not Found"), true},
		{"gh missing", src, fmt.Errorf("gh is not installed — modelith delegates fetching to it"), false},
		{"a rejected credential", src, fmt.Errorf("gh: HTTP 401: Bad credentials"), false},
		{"a forbidden repository", src, fmt.Errorf("gh: HTTP 403: Forbidden"), false},
		{"an unreachable network", src, fmt.Errorf("dial tcp: lookup api.github.com: no such host"), false},
		{"a single-segment path has nothing to lose to the ref",
			Source{Host: HostGitHub, Ref: "main", Path: "payments.modelith.yaml"}, fmt.Errorf("gh: HTTP 404: Not Found"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := splitHint(tc.src, tc.err) != ""; got != tc.want {
				t.Errorf("splitHint offered = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestImport_RefreshIsNotRefusedOverCasing pins that guardTarget compares an
// origin the way GitHub does. Owner and repository names are case-insensitive
// there, and Source.Origin keeps whatever casing the URL was typed with, so a
// byte-for-byte comparison refused a legitimate refresh — and explained it by
// claiming two different models shared the filename.
func TestImport_RefreshIsNotRefusedOverCasing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha},
		"https://github.com/Acme/Billing/blob/main/docs/payments.modelith.yaml"); err != nil {
		t.Fatal(err)
	}
	res, err := importInto(t, dir, &fakeRunner{content: upstream + "  # newer\n", sha: "9" + sha[1:]}, blobURL)
	if err != nil {
		t.Fatalf("a refresh differing only in the origin's casing was refused: %v", err)
	}
	if !res.Replaced {
		t.Error("did not report replacing the earlier copy")
	}
}

// TestImport_RefreshIsNotRefusedOverATrailingSlash pins the other way an origin
// can differ without naming a different repository. A trailing slash is what a
// browser's address bar most readily adds, so a hand-written header carries one;
// refresh already trimmed it when rebuilding the fetch address, and guardTarget
// did not, so the same header updated cleanly under one command and was refused
// as a different model's by another.
func TestImport_RefreshIsNotRefusedOverATrailingSlash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	res, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha}, blobURL)
	if err != nil {
		t.Fatal(err)
	}
	slashed := strings.Replace(readFile(t, res.Path),
		provenance.LinePrefix+"origin: https://github.com/acme/billing\n",
		provenance.LinePrefix+"origin: https://github.com/acme/billing/\n", 1)
	if !strings.Contains(slashed, "billing/\n") {
		t.Fatal("the test did not add a trailing slash to the origin")
	}
	if err := os.WriteFile(res.Path, []byte(slashed), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err = importInto(t, dir, &fakeRunner{content: upstream + "  # newer\n", sha: "9" + sha[1:]}, blobURL)
	if err != nil {
		t.Fatalf("a refresh differing only in a trailing slash was refused: %v", err)
	}
	if !res.Replaced {
		t.Error("did not report replacing the earlier copy")
	}
}

// TestImport_RefusesAMovedPathWithTheRightRemedy pins the other half: the same
// repository at a different path is ambiguous — the model moved, or two models
// there share a basename — and the two remedies differ. Offering only "import
// into a different directory" left the user of a moved model with no way
// forward the message named.
func TestImport_RefusesAMovedPathWithTheRightRemedy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha}, blobURL); err != nil {
		t.Fatal(err)
	}
	_, err := importInto(t, dir, &fakeRunner{content: upstream, sha: sha},
		"https://github.com/acme/billing/blob/main/models/payments.modelith.yaml")
	if err == nil {
		t.Fatal("a second model from the same repository silently replaced the first")
	}
	for _, want := range []string{"If the model moved, delete", "import into a different directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not offer %q: %v", want, err)
		}
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

// --- Azure DevOps tests ---

const adoBlobURL = "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GBmain"

const adoContent = `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
title: Payments
enums:
  PaymentMethod:
    values:
      - name: card
`

const adoCommit = "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21"

func adoRunner(content, sha string) *fakeRunner {
	return &fakeRunner{content: content, sha: sha, ado: true}
}

func importAdoInto(t *testing.T, dir string, r *fakeRunner, url string) (*Result, error) {
	t.Helper()
	return Import(context.Background(), Options{
		URL: url,
		Dir: dir,
		Now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run: r,
	})
}

func TestParseSource_ADO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		ref     string
		want    Source
		wantErr string
	}{
		{
			name: "a browser ADO blob URL with GB branch",
			raw:  adoBlobURL,
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "main",
				RefType: "branch",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "an explicit ref overrides the one in the URL",
			raw:  adoBlobURL,
			ref:  "release/v2",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "release/v2",
				RefType: "",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "an explicit ref resets RefType so the API auto-detects",
			raw:  "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GBmain",
			ref:  "v1.0.0",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "v1.0.0",
				RefType: "",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "GT tag prefix",
			raw:  "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GTv1.0.0",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "v1.0.0",
				RefType: "tag",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "GC commit prefix",
			raw:  "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GC" + adoCommit,
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     adoCommit,
				RefType: "commit",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "an anchor is stripped",
			raw:  adoBlobURL + "&_a=contents",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "main",
				RefType: "branch",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name: "leading slash in path query parameter is stripped",
			raw:  "https://dev.azure.com/myorg/myproject/_git/myrepo?path=/docs/payments.modelith.yaml&version=GBmain",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "main",
				RefType: "branch",
				Path:    "docs/payments.modelith.yaml",
			},
		},
		{
			name:    "no path query parameter",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?version=GBmain",
			wantErr: "names no file inside the repository",
		},
		{
			name:    "not a _git URL",
			raw:     "https://dev.azure.com/myorg/myproject/_wiki/wikis",
			wantErr: "not an Azure DevOps file URL",
		},
		{
			name:    "a traversal segment in URL path is rejected",
			raw:     "https://dev.azure.com/myorg/../_git/myrepo?path=docs/payments.modelith.yaml&version=GBmain",
			wantErr: `has a ".." path segment`,
		},
		{
			name:    "double slash in path query parameter is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs//payments.modelith.yaml&version=GBmain",
			wantErr: `has a "" path segment`,
		},
		{
			name:    "trailing slash in path query parameter is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml/&version=GBmain",
			wantErr: `has a "" path segment`,
		},
		{
			name:    "relative dot traversal in path query parameter is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/./payments.modelith.yaml&version=GBmain",
			wantErr: `has a "." path segment`,
		},
		{
			name:    "parent dot traversal in path query parameter is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/../../secret.yaml&version=GBmain",
			wantErr: `has a ".." path segment`,
		},
		{
			name:    "backslash in path segment is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs\\payments.modelith.yaml&version=GBmain",
			wantErr: `has a "docs\\payments.modelith.yaml" path segment`,
		},
		{
			name:    "untrimmed whitespace in path segment is rejected",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/ payments.modelith.yaml&version=GBmain",
			wantErr: `has a " payments.modelith.yaml" path segment`,
		},
		{
			name:    "legacy visualstudio.com URL offers migration hint to dev.azure.com",
			raw:     "https://myorg.visualstudio.com/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GBmain",
			wantErr: `is a legacy visualstudio.com URL — Azure DevOps moved to dev.azure.com`,
		},
		{
			name:    "no version parameter and no --ref",
			raw:     "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml",
			wantErr: "has no version parameter",
		},
		{
			name: "a bare version with no prefix uses the auto-detect path",
			raw:  "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=main",
			want: Source{
				Host:    HostADO,
				Origin:  "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Owner:   "myorg",
				Project: "myproject",
				Repo:    "myrepo",
				Ref:     "main",
				RefType: "",
				Path:    "docs/payments.modelith.yaml",
			},
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

func TestImport_ADO_StampsAVerifiableCopy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := adoRunner(adoContent, adoCommit)
	res, err := importAdoInto(t, dir, r, adoBlobURL)
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
		Origin:   "https://dev.azure.com/myorg/myproject/_git/myrepo",
		Path:     "docs/payments.modelith.yaml",
		Ref:      "main",
		RefType:  "branch",
		Commit:   adoCommit,
		Imported: "2026-07-27",
		Digest:   provenance.Digest([]byte(adoContent)),
	}
	if *h != want {
		t.Errorf("stamped header = %+v, want %+v", *h, want)
	}
	if ok, got := h.Verify(written); !ok {
		t.Errorf("the freshly written copy does not verify: computed %s", got)
	}

	if !strings.HasPrefix(string(written), "# yaml-language-server:") {
		t.Error("the editor directive is no longer the first line")
	}
	if !strings.Contains(string(written), "  PaymentMethod:\n") {
		t.Error("the model content did not survive the stamp")
	}
}

func TestImport_ADO_CallsAzWithTheExpectedEndpoints(t *testing.T) {
	t.Parallel()

	r := adoRunner(adoContent, adoCommit)
	if _, err := importAdoInto(t, t.TempDir(), r, adoBlobURL); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("want 2 az calls, got %d: %+v", len(r.calls), r.calls)
	}
	// Find the --uri and --resource arguments in each call.
	var foundContent, foundCommit, foundResource int
	for _, call := range r.calls {
		for i, a := range call {
			if a == "--resource" && i+1 < len(call) && call[i+1] == adoResourceID {
				foundResource++
			}
			if a == "--uri" && i+1 < len(call) {
				uri := call[i+1]
				if strings.Contains(uri, "/items") {
					foundContent++
					// Without download=true the Items API returns a JSON
					// GitItem, and the import cannot parse the model out of it.
					if !strings.Contains(uri, "download=true") {
						t.Errorf("the items request omits download=true, so the API returns JSON metadata rather than the model: %s", uri)
					}
				}
				if strings.Contains(uri, "/commits") {
					foundCommit++
				}
			}
		}
	}
	if foundResource != 2 {
		t.Errorf("want 2 --resource flags, got %d", foundResource)
	}
	if foundContent == 0 {
		t.Error("no az rest call with /items endpoint")
	}
	if foundCommit == 0 {
		t.Error("no az rest call with /commits endpoint")
	}
}

// TestImport_ADO_TagUsesVersionTypeTag pins that a GT (tag) URL produces
// versionType=tag in the API call, not the hardcoded "branch".
func TestImport_ADO_TagUsesVersionTypeTag(t *testing.T) {
	t.Parallel()

	r := adoRunner(adoContent, adoCommit)
	tagURL := "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GTv1.0.0"
	if _, err := importAdoInto(t, t.TempDir(), r, tagURL); err != nil {
		t.Fatal(err)
	}
	for _, call := range r.calls {
		for i, a := range call {
			if a == "--uri" && i+1 < len(call) {
				uri := call[i+1]
				if strings.Contains(uri, "/items") && !strings.Contains(uri, "versionType=tag") {
					t.Errorf("tag URL should produce versionType=tag, got %q", uri)
				}
				if strings.Contains(uri, "/commits") && !strings.Contains(uri, "versionType=tag") {
					t.Errorf("tag URL should produce versionType=tag in commits call, got %q", uri)
				}
			}
		}
	}
}

// TestImport_ADO_CommitUsesVersionTypeCommit pins that a GC (commit) URL
// produces versionType=commit in the API call.
func TestImport_ADO_CommitUsesVersionTypeCommit(t *testing.T) {
	t.Parallel()

	r := adoRunner(adoContent, adoCommit)
	commitURL := "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml&version=GC" + adoCommit
	if _, err := importAdoInto(t, t.TempDir(), r, commitURL); err != nil {
		t.Fatal(err)
	}
	for _, call := range r.calls {
		for i, a := range call {
			if a == "--uri" && i+1 < len(call) {
				uri := call[i+1]
				if strings.Contains(uri, "/items") && !strings.Contains(uri, "versionType=commit") {
					t.Errorf("commit URL should produce versionType=commit, got %q", uri)
				}
			}
		}
	}
}

// TestImport_ADO_OverrideRefOmitsVersionType pins that when --ref overrides
// the URL's ref, the API call omits versionType so ADO auto-detects.
func TestImport_ADO_OverrideRefOmitsVersionType(t *testing.T) {
	t.Parallel()

	r := adoRunner(adoContent, adoCommit)
	// URL has GBmain (branch), but --ref overrides to a tag-like value.
	url := adoBlobURL
	_, err := Import(context.Background(), Options{
		URL: url,
		Dir: t.TempDir(),
		Ref: "v1.0.0",
		Now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run: r,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range r.calls {
		for i, a := range call {
			if a == "--uri" && i+1 < len(call) {
				uri := call[i+1]
				if strings.Contains(uri, "versionType") {
					t.Errorf("--ref override should omit versionType, got %q", uri)
				}
				if !strings.Contains(uri, "version=v1.0.0") {
					t.Errorf("--ref override should use the override value in version=, got %q", uri)
				}
			}
		}
	}
}

func TestImport_ADO_RejectsAlreadyVendored(t *testing.T) {
	t.Parallel()

	r := adoRunner("# modelith-origin: https://dev.azure.com/other/proj/_git/repo\n"+adoContent, adoCommit)
	_, err := importAdoInto(t, t.TempDir(), r, adoBlobURL)
	if err == nil || !strings.Contains(err.Error(), "reads it as somebody else's copy") {
		t.Fatalf("want 'reads it as somebody else's copy', got %v", err)
	}
}

func TestImport_ADO_Rejections(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		runner  *fakeRunner
		url     string
		wantErr string
	}{
		{
			name:    "a file that is not a domain model",
			runner:  adoRunner("kind: SomethingElse\nversion: v1\n", adoCommit),
			url:     adoBlobURL,
			wantErr: `it declares kind "SomethingElse"`,
		},
		{
			name:    "a file with no kind at all",
			runner:  adoRunner("title: Payments\n", adoCommit),
			url:     adoBlobURL,
			wantErr: "it declares no kind",
		},
		{
			name:    "a path with no commits at that ref (empty commit sha)",
			runner:  adoRunner(adoContent, ""),
			url:     adoBlobURL,
			wantErr: "no commit touching",
		},
		{
			name:    "a path with no commits at that ref (null commit sha)",
			runner:  adoRunner(adoContent, "null"),
			url:     adoBlobURL,
			wantErr: "no commit touching",
		},
		{
			name:    "az refusing the fetch",
			runner:  &fakeRunner{content: adoContent, sha: adoCommit, ado: true, fail: "/items"},
			url:     adoBlobURL,
			wantErr: "HTTP 404",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := importAdoInto(t, dir, tc.runner, tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestImport_ADO_RejectsEmptyRef pins that an ADO URL without a version
// parameter and no --ref is rejected with a clear message, rather than
// silently stamping a header with an empty ref.
func TestImport_ADO_RejectsEmptyRef(t *testing.T) {
	t.Parallel()

	url := "https://dev.azure.com/myorg/myproject/_git/myrepo?path=docs/payments.modelith.yaml"
	_, err := Import(context.Background(), Options{
		URL: url,
		Dir: t.TempDir(),
		Now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run: adoRunner(adoContent, adoCommit),
	})
	if err == nil {
		t.Fatal("want an error for a URL with no version parameter, got nil")
	}
	if !strings.Contains(err.Error(), "has no version parameter") {
		t.Errorf("want 'has no version parameter' in error, got: %v", err)
	}
}

func TestSplitHint_ADO(t *testing.T) {
	t.Parallel()

	src := Source{
		Host:    HostADO,
		Project: "myproject",
		Ref:     "main",
		Path:    "docs/payments.modelith.yaml",
	}
	if got := splitHint(src, fmt.Errorf("HTTP 404: Not Found")); got != "" {
		t.Errorf("splitHint for ADO source should be empty, got %q", got)
	}
}

// --- Timeout decorator tests ---

// runnerFunc adapts a func to the Runner interface for tests.
type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

// hungRunner blocks every call until the context Import gave it expires, the
// way a stalled az/gh process behaves under exec.CommandContext.
type hungRunner struct{ calls int }

func (h *hungRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	h.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

// ctxRecordingRunner records the context each call received, so a test can
// assert what Import delivered: a deadline when Timeout is set, the caller's
// bare ctx when it is zero.
type ctxRecordingRunner struct {
	fakeRunner
	ctxs []context.Context
}

func (r *ctxRecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.ctxs = append(r.ctxs, ctx)
	return r.fakeRunner.Run(ctx, name, args...)
}

// TestTimeoutRunner_ReturnsAClearErrorOnDeadline pins the decorator's contract:
// a hung command becomes an error that names the command and the bound and says
// how to raise it, and that never echoes the argv — defense in depth, since the
// argv can carry URIs a caller would not want duplicated into error logs.
func TestTimeoutRunner_ReturnsAClearErrorOnDeadline(t *testing.T) {
	t.Parallel()

	inner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	r := timeoutRunner{inner: inner, timeout: 10 * time.Millisecond}

	_, err := r.Run(context.Background(), "az", "rest", "--uri", "https://dev.azure.com/secret/org/_apis/items")
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	for _, want := range []string{"az", "10ms", "--timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %q: %v", want, err)
		}
	}
	for _, leaked := range []string{"rest", "--uri", "dev.azure.com/secret"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("the error echoes the argv (%q): %v", leaked, err)
		}
	}
}

// TestTimeoutRunner_ReportsAKilledChildAsTheBound pins the issue #3 fix: when
// the bound fires, ExecRunner SIGKILLs the process group and cmd.Output
// returns *exec.ExitError ("signal: killed"), which errors.Is against
// DeadlineExceeded / ErrWaitDelay does not match. The friendly message must
// fire anyway — and must not echo the argv, the URI the command was fetching.
func TestTimeoutRunner_ReportsAKilledChildAsTheBound(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}

	r := timeoutRunner{inner: ExecRunner{}, timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := r.Run(context.Background(), "sleep", "30")
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	for _, want := range []string{"sleep", "did not finish within", "--timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %q: %v", want, err)
		}
	}
	for _, leaked := range []string{"signal: killed", "sleep 30"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("the error leaks %q: %v", leaked, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Run blocked %v after the bound — the deadline did not fire", elapsed)
	}
}

// TestTimeoutRunner_ParentCancelIsNotReportedAsTimeout pins the review finding
// on 3f88c53: a caller canceling the context (e.g. a future Ctrl+C handler)
// must not be relabeled as a timeout. The bound is deliberately far away, so
// only the parent cancel can stop the child — the error path is otherwise
// identical to a timeout (process-group SIGKILL, *exec.ExitError), and the
// context is what tells the two apart: context.Canceled is not a deadline the
// user can raise with --timeout.
func TestTimeoutRunner_ParentCancelIsNotReportedAsTimeout(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	r := timeoutRunner{inner: ExecRunner{}, timeout: time.Minute}
	start := time.Now()
	_, err := r.Run(ctx, "sleep", "30")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, misleading := range []string{"did not finish within", "--timeout"} {
		if strings.Contains(err.Error(), misleading) {
			t.Errorf("a caller cancel is misreported as a timeout (%q): %v", misleading, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Run blocked %v after the cancel — the child was not killed", elapsed)
	}
}

// TestImport_TimeoutIsPerCommand pins that the bound applies to each command
// individually, not to the whole import: a whole-import budget could let the
// first call consume everything and fail on the second. A per-command deadline
// fails at the first hung call, which is the one the user is waiting on.
func TestImport_TimeoutIsPerCommand(t *testing.T) {
	t.Parallel()

	hung := &hungRunner{}
	_, err := Import(context.Background(), Options{
		URL:     blobURL,
		Dir:     t.TempDir(),
		Now:     time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run:     hung,
		Timeout: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "gh did not finish within") {
		t.Errorf("the error should name the first command (gh), got: %v", err)
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("the error should say how to adjust the bound, got: %v", err)
	}
	if hung.calls != 1 {
		t.Errorf("the first hung call consumed the whole import; got %d calls, want 1", hung.calls)
	}
}

// TestImport_ADO_TimeoutBoundsTheAzCall pins the fix this feature exists for:
// a stalled az rest becomes a fast, actionable error naming az, not a silent
// 75s wait on the commits-with-path query.
func TestImport_ADO_TimeoutBoundsTheAzCall(t *testing.T) {
	t.Parallel()

	hung := &hungRunner{}
	_, err := Import(context.Background(), Options{
		URL:     adoBlobURL,
		Dir:     t.TempDir(),
		Now:     time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run:     hung,
		Timeout: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "az did not finish within") {
		t.Errorf("the error should name az, got: %v", err)
	}
	if hung.calls != 1 {
		t.Errorf("the first az call should fail alone; got %d calls", hung.calls)
	}
}

// TestImport_ZeroTimeoutPassesTheCallerContextThrough pins that Options.Timeout
// zero (the default) leaves the runner's context untouched: no deadline is
// added, so the decorator is a no-op and existing behavior is unchanged.
func TestImport_ZeroTimeoutPassesTheCallerContextThrough(t *testing.T) {
	t.Parallel()

	rec := &ctxRecordingRunner{fakeRunner: fakeRunner{content: upstream, sha: sha}}
	if _, err := Import(context.Background(), Options{
		URL:     blobURL,
		Dir:     t.TempDir(),
		Now:     time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run:     rec,
		Timeout: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if len(rec.ctxs) != 2 {
		t.Fatalf("want 2 calls, got %d", len(rec.ctxs))
	}
	for i, ctx := range rec.ctxs {
		if _, ok := ctx.Deadline(); ok {
			t.Errorf("call %d received a deadline; Timeout 0 must pass the caller's ctx through", i)
		}
	}
}

// TestImport_WithTimeoutBoundsEachCall pins that Timeout > 0 applies the
// decorator to every delegated command, and that a command that finishes within
// the bound still succeeds.
func TestImport_WithTimeoutBoundsEachCall(t *testing.T) {
	t.Parallel()

	rec := &ctxRecordingRunner{fakeRunner: fakeRunner{content: upstream, sha: sha}}
	if _, err := Import(context.Background(), Options{
		URL:     blobURL,
		Dir:     t.TempDir(),
		Now:     time.Date(2026, 7, 27, 12, 0, 0, 0, time.Local),
		Run:     rec,
		Timeout: time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	for i, ctx := range rec.ctxs {
		d, ok := ctx.Deadline()
		if !ok {
			t.Errorf("call %d received no deadline; Timeout %v should bound each call", i, time.Minute)
			continue
		}
		if time.Until(d) > time.Minute {
			t.Errorf("call %d deadline %v is later than the %v bound", i, d, time.Minute)
		}
	}
}

// TestExecRunner_KillsTheWholeProcessGroup pins the fix for the macOS stall
// (issue #2): CommandContext's default kill sends SIGKILL only to the direct
// child, so a CLI that spawned a helper leaves that helper holding the stdout
// pipe — Wait then blocks past the deadline. Killing the process group closes
// the pipe and the call returns. Here the direct child (sh) is waiting on a
// backgrounded sleep: killing only sh would leave sleep holding the pipe for
// its full 30s.
func TestExecRunner_KillsTheWholeProcessGroup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := (ExecRunner{}).Run(ctx, "sh", "-c", "sleep 30 & wait")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Run blocked %v after the deadline — the process group was not killed", elapsed)
	}
}

// TestExecRunner_WaitDelayBoundsAPipeHeldByAStrayChild pins the belt-and-
// braces half of the fix: even if a helper escapes the process group, the
// stdout pipe it inherited cannot hold Wait hostage past WaitDelay. The shell
// exits immediately; the backgrounded sleep inherits the stdout pipe and keeps
// it open for 30s. WaitDelay must make Run return long before that.
func TestExecRunner_WaitDelayBoundsAPipeHeldByAStrayChild(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are unix-only")
	}

	start := time.Now()
	_, err := (ExecRunner{}).Run(context.Background(), "sh", "-c", "sleep 30 &")
	if err == nil {
		t.Fatal("expected an error (WaitDelay abandoned the held pipe), got nil")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Run blocked %v on a pipe held by a stray child", elapsed)
	}
}
