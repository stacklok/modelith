package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// vendored is a stamped file: the editor directive, then the header, then a
// model. Tests that need the unstamped content use plain.
//
// Its origin is Azure DevOps, the one host whose header records a ref type, so
// the fixture is a header that can actually occur. A GitHub origin with the key
// is a defect the validator reports (see the ADR-0019 test below).
const vendored = `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
# modelith-vendored: DO NOT EDIT — this file is a copy. Change it at its origin.
# modelith-fetch: git
# modelith-origin: https://dev.azure.com/myorg/myproject/_git/myrepo
# modelith-path: docs/payments.modelith.yaml
# modelith-ref: main
# modelith-ref-type: branch
# modelith-commit: 4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21
# modelith-imported: 2026-07-27
# modelith-digest: sha256:0000000000000000000000000000000000000000000000000000000000000000
kind: DomainModel
version: v1
title: Payments
`

const plain = `# yaml-language-server: $schema=https://modelith.sh/schema/domain-model/v1.json
kind: DomainModel
version: v1
title: Payments
`

func TestPresent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"a stamped file", vendored, true},
		{"a model this repo owns", plain, false},
		{"an ordinary comment that only looks like one", "#modelith-fetch: git\nkind: DomainModel\n", false},
		{"an indented provenance line is an ordinary comment", "  # modelith-fetch: git\nkind: DomainModel\n", false},
		{"a broken header is still a header", "# modelith-nonsense: x\nkind: DomainModel\n", true},
		{"a header displaced below the model content is still a header", plain + "# modelith-fetch: git\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Present([]byte(tc.src)); got != tc.want {
				t.Errorf("Present() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParse_Valid(t *testing.T) {
	t.Parallel()

	// A GitHub header must not carry the ref-type key (ADR-0019), so the second
	// shape is the fixture with a GitHub origin and the line dropped. Each step
	// is checked to have changed the string: a future fixture edit that stopped
	// one of them matching would leave this case a silent duplicate of the ADO
	// one, and the test would keep passing while covering nothing.
	withGitHubOrigin := strings.Replace(vendored, "https://dev.azure.com/myorg/myproject/_git/myrepo", "https://github.com/stacklok/some-repo", 1)
	if withGitHubOrigin == vendored {
		t.Fatal("the test did not rewrite the origin, so the GitHub case would duplicate the ADO one")
	}
	github := strings.Replace(withGitHubOrigin, LinePrefix+"ref-type: branch\n", "", 1)
	if github == withGitHubOrigin {
		t.Fatal("the test did not drop the ref-type line, so the GitHub case would duplicate the ADO one")
	}

	cases := []struct {
		name string
		src  string
		want Header
	}{
		{
			name: "an Azure DevOps header",
			src:  vendored,
			want: Header{
				Vendored: Banner,
				Fetch:    "git",
				Origin:   "https://dev.azure.com/myorg/myproject/_git/myrepo",
				Path:     "docs/payments.modelith.yaml",
				Ref:      "main",
				RefType:  "branch",
				Commit:   "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21",
				Imported: "2026-07-27",
				Digest:   "sha256:" + strings.Repeat("0", 64),
			},
		},
		{
			name: "a GitHub header",
			src:  github,
			want: Header{
				Vendored: Banner,
				Fetch:    "git",
				Origin:   "https://github.com/stacklok/some-repo",
				Path:     "docs/payments.modelith.yaml",
				Ref:      "main",
				Commit:   "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21",
				Imported: "2026-07-27",
				Digest:   "sha256:" + strings.Repeat("0", 64),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, problems := Parse([]byte(tc.src))
			if len(problems) != 0 {
				t.Fatalf("unexpected problems: %+v", problems)
			}
			if h == nil {
				t.Fatal("Parse returned no header")
			}
			if *h != tc.want {
				t.Errorf("Parse() = %+v, want %+v", *h, tc.want)
			}
		})
	}
}

func TestParse_NoHeader(t *testing.T) {
	t.Parallel()

	h, problems := Parse([]byte(plain))
	if h != nil || problems != nil {
		t.Errorf("Parse() = %+v, %+v; want nil, nil", h, problems)
	}
}

func TestParse_Problems(t *testing.T) {
	t.Parallel()

	// header builds a stamped file with the named lines replaced or dropped, so
	// each case differs from a valid header in exactly one way.
	header := func(edits map[string]string) string {
		var b strings.Builder
		b.WriteString("# yaml-language-server: $schema=x\n")
		for _, key := range keyOrder {
			value := map[string]string{
				"vendored": Banner,
				"fetch":    "git",
				"origin":   "https://dev.azure.com/myorg/myproject/_git/myrepo",
				"path":     "docs/payments.modelith.yaml",
				"ref":      "main",
				"ref-type": "branch",
				"commit":   "4f2c1e9",
				"imported": "2026-07-27",
				"digest":   "sha256:" + strings.Repeat("0", 64),
			}[key]
			if edit, ok := edits[key]; ok {
				value = edit
			}
			if value == "" {
				continue
			}
			b.WriteString(LinePrefix + key + ": " + value + "\n")
		}
		b.WriteString("kind: DomainModel\n")
		return b.String()
	}

	cases := []struct {
		name     string
		src      string
		wantLine int
		contains string
	}{
		{
			name:     "an unknown key names the ones that exist",
			src:      "# modelith-surface: sha256:x\n" + plain,
			wantLine: 1,
			contains: `unknown provenance key "# modelith-surface"`,
		},
		{
			name:     "a duplicate key names both lines",
			src:      "# modelith-ref: main\n# modelith-ref: other\n" + plain,
			wantLine: 2,
			contains: `appears twice, on lines 1 and 2`,
		},
		{
			name:     "a missing required key is reported without a line",
			src:      header(map[string]string{"commit": ""}),
			wantLine: 0,
			contains: `missing "# modelith-commit"`,
		},
		{
			name:     "an unknown fetch method points at the issue tracker",
			src:      header(map[string]string{"fetch": "carrier-pigeon"}),
			wantLine: 3,
			contains: `unknown fetch method "carrier-pigeon"`,
		},
		{
			name:     "a malformed digest names the shape",
			src:      header(map[string]string{"digest": "sha256:nope"}),
			wantLine: 10,
			contains: "sha256:<64 hex digits>",
		},
		{
			name:     "a ref-type outside the closed set is reported",
			src:      header(map[string]string{"ref-type": "sausage"}),
			wantLine: 7,
			contains: `provenance ref-type "sausage" is not one of`,
		},
		{
			name:     "a provenance line below the model content is misplaced",
			src:      "# modelith-fetch: git\n" + plain + "# modelith-origin: https://github.com/stacklok/some-repo\n",
			wantLine: 6,
			contains: "below the leading comment block",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, problems := Parse([]byte(tc.src))
			var found *Problem
			for i := range problems {
				if strings.Contains(problems[i].Message, tc.contains) {
					found = &problems[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no problem containing %q; got %+v", tc.contains, problems)
			}
			if found.Line != tc.wantLine {
				t.Errorf("problem on line %d, want %d: %s", found.Line, tc.wantLine, found.Message)
			}
		})
	}
}

// TestADR_0015_DigestCoversTheFileWithoutItsHeader pins ADR-0015's digest definition.
//
// The expectation is built by hand — the header lines written out and removed
// by the test itself, then hashed with crypto/sha256 directly — rather than by
// calling Strip. A test that reuses the implementation's own helper would agree
// with it however wrong the helper was, which is exactly how two HIGH bugs got
// past CI in the renderer work.
func TestADR_0015_DigestCoversTheFileWithoutItsHeader(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte(plain))
	want := "sha256:" + hex.EncodeToString(sum[:])

	if got := Digest([]byte(plain)); got != want {
		t.Errorf("Digest(plain) = %s, want %s", got, want)
	}
	if got := Digest([]byte(vendored)); got != want {
		t.Errorf("Digest(vendored) = %s, want %s — the header must not change the digest of the file it is stamped into", got, want)
	}
}

// TestADR_0015_UnknownFetchMethodIsAnError pins that the fetch method is a
// closed set. A header naming a method this build does not implement is
// reported rather than being read as if it were git — which would verify a
// digest against keys that mean something else — and the diagnostic asks for an
// issue, because a real user wanting another transport is what justifies
// writing one.
func TestADR_0015_UnknownFetchMethodIsAnError(t *testing.T) {
	t.Parallel()

	src := strings.Replace(vendored, LinePrefix+"fetch: git", LinePrefix+"fetch: carrier-pigeon", 1)
	_, problems := Parse([]byte(src))

	var found bool
	for _, p := range problems {
		if strings.Contains(p.Message, `unknown fetch method "carrier-pigeon"`) {
			found = true
			if !strings.Contains(p.Message, issuesHint) {
				t.Errorf("the diagnostic does not ask for an issue: %s", p.Message)
			}
			if !strings.Contains(p.Message, `"git"`) {
				t.Errorf("the diagnostic does not name the methods that work: %s", p.Message)
			}
		}
	}
	if !found {
		t.Fatalf("no unknown-method problem; got %+v", problems)
	}
}

const issuesHint = "https://github.com/stacklok/modelith/issues"

// TestADR_0019_RefTypeIsOnlyValidOnAnADOOrigin pins the other half of the
// ADR-0019 header rule — the half that lives on the reading side. The key is
// recorded only for Azure DevOps, whose API needs the type; GitHub's does not,
// so a header that carries the key on a GitHub (or any other) origin is a
// defect, not a value to ignore. Before this, such a header parsed clean and
// `deps update` silently dropped the key.
func TestADR_0019_RefTypeIsOnlyValidOnAnADOOrigin(t *testing.T) {
	t.Parallel()

	// A valid file is the fixture itself. Everything else differs from it in
	// exactly one way.
	if _, problems := Parse([]byte(vendored)); len(problems) != 0 {
		t.Fatalf("the ADO fixture is not valid: %+v", problems)
	}

	github := strings.Replace(vendored, "https://dev.azure.com/myorg/myproject/_git/myrepo", "https://github.com/acme/billing", 1)
	_, problems := Parse([]byte(github))
	var found *Problem
	for i := range problems {
		if strings.Contains(problems[i].Message, "recorded only for a dev.azure.com origin") {
			found = &problems[i]
		}
	}
	if found == nil {
		t.Fatalf("a GitHub origin carrying a ref-type parsed clean; want a problem, got %+v", problems)
	}
	if !strings.Contains(found.Message, `"https://github.com/acme/billing"`) {
		t.Errorf("the problem does not name the origin it read: %s", found.Message)
	}
	if found.Line == 0 {
		t.Errorf("the problem does not point at the ref-type line: %+v", found)
	}

	// And the key is still optional: a GitHub header without it stays valid,
	// which is what keeps a pre-existing GitHub copy byte-identical.
	noKey := strings.Replace(github, LinePrefix+"ref-type: branch\n", "", 1)
	if _, problems := Parse([]byte(noKey)); len(problems) != 0 {
		t.Errorf("a GitHub header without a ref-type was rejected: %+v", problems)
	}

	// An ADO origin may name its port explicitly, and the port is not part of
	// the host: the same origin with and without one must be read as the same
	// host, so a valid ADO header carrying a port parses clean.
	ported := strings.Replace(vendored, "https://dev.azure.com/myorg/myproject/_git/myrepo", "https://dev.azure.com:443/myorg/myproject/_git/myrepo", 1)
	if ported == vendored {
		t.Fatal("the test did not rewrite the origin to carry a port")
	}
	if _, problems := Parse([]byte(ported)); len(problems) != 0 {
		t.Fatalf("an ADO origin carrying an explicit port was rejected: %+v", problems)
	}
}

// TestOriginHost pins the normalizer the provenance validator and the deps
// transport share. A host is case-insensitive and may carry a leading "www.";
// a port is not part of the host, so an origin written with an explicit one
// names the same site as the one without — which is what keeps `modelith lint`
// and `modelith deps check` from disagreeing about a hand-written origin.
func TestOriginHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		origin string
		want   string
	}{
		{"a bare ADO origin", "https://dev.azure.com/myorg/x", "dev.azure.com"},
		{"an explicit port is dropped", "https://dev.azure.com:443/myorg/x", "dev.azure.com"},
		{"a non-default port is dropped too", "https://dev.azure.com:8443/myorg/x", "dev.azure.com"},
		{"case and the www prefix are normalized away", "https://WWW.GitHub.com/a/b", "github.com"},
		{"the plain GitHub origin", "https://github.com/a/b", "github.com"},
		{"a value that parses as a URL with no host", "/just/a/path", ""},
		{"a value that does not parse as a URL names no host", "not a url", ""},
		{"the empty string names no host", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := OriginHost(tc.origin); got != tc.want {
				t.Errorf("OriginHost(%q) = %q, want %q", tc.origin, got, tc.want)
			}
		})
	}

	if got := NormalizeHost("Dev.Azure.com"); got != "dev.azure.com" {
		t.Errorf("NormalizeHost(%q) = %q, want %q", "Dev.Azure.com", got, "dev.azure.com")
	}
}

func TestDigest_ChangesWithTheContent(t *testing.T) {
	t.Parallel()

	edited := strings.Replace(vendored, "title: Payments", "title: Payment", 1)
	if Digest([]byte(edited)) == Digest([]byte(vendored)) {
		t.Error("editing the model left the digest unchanged")
	}
}

// TestDigest_IgnoresEveryHeaderChange pins that the digest is blind to the
// header and to nothing else: rewriting each key in turn must not move it,
// because the header records a value derived from the rest of the file.
func TestDigest_IgnoresEveryHeaderChange(t *testing.T) {
	t.Parallel()

	base := Digest([]byte(vendored))
	for _, key := range keyOrder {
		src := strings.Replace(vendored, LinePrefix+key+": ", LinePrefix+key+": changed-", 1)
		if src == vendored {
			t.Fatalf("test did not rewrite %q", key)
		}
		if got := Digest([]byte(src)); got != base {
			t.Errorf("rewriting %q moved the digest to %s", key, got)
		}
	}
}

// TestDigest_CoversALineBelowTheHeader pins the other half of the strip rule:
// only the leading comment block is header, so a "# modelith-" line anywhere
// below it is content the digest covers. Stripping it there would make an edit
// that adds or removes one invisible to drift detection — and would make a model
// this repository owns, which happened to carry such a comment, read as
// somebody else's copy.
func TestDigest_CoversALineBelowTheHeader(t *testing.T) {
	t.Parallel()

	added := vendored + "# modelith-fetch: git\n"
	if Digest([]byte(added)) == Digest([]byte(vendored)) {
		t.Error("a provenance-looking line added below the model left the digest unchanged")
	}
	if !strings.Contains(string(Strip([]byte(added))), "# modelith-fetch: git") {
		t.Error("Strip removed a line below the leading comment block")
	}
}

// TestPresent_ADisplacedHeaderStaysLoud pins the reason Present asks a broader
// question than Parse and Strip do. Prepending one non-comment line to a
// vendored file puts its whole header below the leading comment block. Answering
// the narrow question there would call the file this repository's own work, and
// every consequence of that answer is silent: no digest check, completeness
// gaps about somebody else's document, and a `deps import` that refuses to
// repair the file because it "carries no provenance header".
func TestPresent_ADisplacedHeaderStaysLoud(t *testing.T) {
	t.Parallel()

	displaced := "---\n" + vendored
	if !Present([]byte(displaced)) {
		t.Fatal("a vendored file with its header displaced read as this repository's own work")
	}
	h, problems := Parse([]byte(displaced))
	if h == nil {
		t.Fatal("Parse returned no header for a displaced one")
	}
	var misplaced, missing int
	for _, p := range problems {
		switch {
		case strings.Contains(p.Message, "below the leading comment block"):
			misplaced++
		case strings.Contains(p.Message, "is missing"):
			missing++
		}
	}
	if misplaced != len(keyOrder) {
		t.Errorf("reported %d misplaced lines, want one per key (%d)", misplaced, len(keyOrder))
	}
	if missing == 0 {
		t.Error("no key was reported missing, so the header read as complete")
	}
	// Nothing was stripped, so the digest cannot match what the header records.
	if string(Strip([]byte(displaced))) != displaced {
		t.Error("Strip removed a line from below the leading comment block")
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	h := &Header{Digest: Digest([]byte(plain))}
	if ok, _ := h.Verify([]byte(vendored)); !ok {
		t.Error("a stamped copy of the same content did not verify")
	}
	edited := strings.Replace(vendored, "title: Payments", "title: Payment", 1)
	ok, got := h.Verify([]byte(edited))
	if ok {
		t.Error("an edited copy verified")
	}
	if got == h.Digest {
		t.Error("Verify reported the recorded digest as the one it computed")
	}
}

func TestStamp(t *testing.T) {
	t.Parallel()

	h := &Header{
		Vendored: Banner,
		Fetch:    "git",
		Origin:   "https://github.com/stacklok/some-repo",
		Path:     "docs/payments.modelith.yaml",
		Ref:      "main",
		Commit:   "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21",
		Imported: "2026-07-27",
		Digest:   Digest([]byte(plain)),
	}

	t.Run("below a yaml-language-server line", func(t *testing.T) {
		t.Parallel()
		got := string(Stamp([]byte(plain), h))
		lines := strings.Split(got, "\n")
		if !strings.HasPrefix(lines[0], "# yaml-language-server:") {
			t.Errorf("first line is %q, want the editor directive", lines[0])
		}
		if !strings.HasPrefix(lines[1], LinePrefix+"vendored:") {
			t.Errorf("second line is %q, want the vendored banner", lines[1])
		}
		if !strings.Contains(got, "kind: DomainModel") {
			t.Error("the model content did not survive stamping")
		}
	})

	t.Run("at the top when there is no directive", func(t *testing.T) {
		t.Parallel()
		src := "kind: DomainModel\nversion: v1\n"
		got := string(Stamp([]byte(src), h))
		if !strings.HasPrefix(got, LinePrefix+"vendored:") {
			t.Errorf("stamped file starts %q", got[:40])
		}
		if !strings.HasSuffix(got, src) {
			t.Error("the model content did not survive stamping unchanged")
		}
	})

	t.Run("a stamped file parses back to the header", func(t *testing.T) {
		t.Parallel()
		back, problems := Parse(Stamp([]byte(plain), h))
		if len(problems) != 0 {
			t.Fatalf("stamping produced a header with problems: %+v", problems)
		}
		if *back != *h {
			t.Errorf("round trip gave %+v, want %+v", *back, *h)
		}
	})

	t.Run("the stamped file verifies against its own digest", func(t *testing.T) {
		t.Parallel()
		stamped := Stamp([]byte(plain), h)
		parsed, _ := Parse(stamped)
		if ok, got := parsed.Verify(stamped); !ok {
			t.Errorf("a freshly stamped file does not verify: recorded %s, computed %s", parsed.Digest, got)
		}
	})
}

func TestStrip(t *testing.T) {
	t.Parallel()

	if got := string(Strip([]byte(vendored))); got != plain {
		t.Errorf("Strip() = %q, want %q", got, plain)
	}
	if got := string(Strip([]byte(plain))); got != plain {
		t.Errorf("Strip() changed a file with no header: %q", got)
	}
}
