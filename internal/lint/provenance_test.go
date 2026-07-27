package lint

import (
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/provenance"
)

// stamp returns src as a vendored copy: a well-formed provenance header whose
// digest matches the content it is stamped into.
func stamp(t *testing.T, src string) string {
	t.Helper()
	h := &provenance.Header{
		Vendored: provenance.Banner,
		Fetch:    "git",
		Origin:   "https://github.com/stacklok/some-repo",
		Path:     "docs/payments.modelith.yaml",
		Ref:      "main",
		Commit:   "4f2c1e9c8b3ad0e5f71b2c9a6d4e8f30ab5c7d21",
		Imported: "2026-07-27",
		Digest:   provenance.Digest([]byte(src)),
	}
	return string(provenance.Stamp([]byte(src), h))
}

// gappy is a model with one finding in each layer that a vendored file's status
// could plausibly touch: a semantic error (a relationship to an entity that is
// not defined), a semantic warning (a PascalCase type naming no enum), and
// completeness gaps (no invariants, and no scenario exercising the entity).
const gappy = `kind: DomainModel
version: v1
entities:
  Visit:
    definition: One car's stay in the garage.
    attributes:
      - name: settledWith
        type: PaymentMethod
    relationships:
      - entity: Nonexistent
        cardinality: "n:1"
        ownership: referenced
`

func lintSource(t *testing.T, src string, files fakeFiles) *Result {
	t.Helper()
	res, err := Run(importerPath, []byte(src), files)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestVendored_SuppressesCompletenessAndNothingElse pins ADR-0015's rule as a
// difference rather than as a hand-written expectation: the same model is linted
// twice, once as this repository's own work and once as a vendored copy, and
// every finding that disappears must be a completeness finding.
//
// Written this way the test cannot be satisfied by a suppression that is too
// broad. A rule that also dropped semantic warnings would pass a test that
// merely listed what a vendored file should report.
func TestVendored_SuppressesCompletenessAndNothingElse(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": ""}
	own := lintSource(t, gappy, files)
	vendored := lintSource(t, stamp(t, gappy), files)

	byKey := func(f Finding) string { return string(f.Category) + "|" + f.Path + "|" + f.Message }

	kept := map[string]bool{}
	for _, f := range vendored.Findings {
		kept[byKey(f)] = true
	}

	var droppedCompleteness int
	for _, f := range own.Findings {
		if kept[byKey(f)] {
			continue
		}
		if f.Category != CategoryCompleteness {
			t.Errorf("a %s finding was suppressed on a vendored file: %s: %s", f.Category, f.Path, f.Message)
			continue
		}
		droppedCompleteness++
	}
	if droppedCompleteness == 0 {
		t.Fatal("the fixture produced no completeness findings, so this proves nothing")
	}

	// And nothing new appeared: a well-formed header is not itself a finding.
	ownKeys := map[string]bool{}
	for _, f := range own.Findings {
		ownKeys[byKey(f)] = true
	}
	for _, f := range vendored.Findings {
		if !ownKeys[byKey(f)] {
			t.Errorf("vendoring added a finding: %s: %s", f.Path, f.Message)
		}
	}

	// The semantic error and warning both survive, which is the half of the
	// rule that a broad suppression would break.
	var errs, warns int
	for _, f := range vendored.Findings {
		switch f.Severity {
		case SeverityError:
			errs++
		case SeverityWarning:
			warns++
		}
	}
	if errs == 0 || warns == 0 {
		t.Errorf("vendored file reported %d error(s) and %d warning(s); both layers should still run", errs, warns)
	}
}

// TestVendored_ImportsRaiseNothing covers the second suppression and the
// downstream consequence that makes it worth anything: an unresolvable import
// is silent, and so is the reference that resolves through it. Without the
// second half, every vendored model that has imports of its own is broken here.
func TestVendored_ImportsRaiseNothing(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": ""}
	src := importer([]string{`"./nowhere.modelith.yaml"`}, "nowhere.PaymentMethod")

	own := lintSource(t, src, files)
	if len(importFindings(own.Findings)) == 0 {
		t.Fatal("the unstamped fixture reported nothing, so this proves nothing")
	}

	vendored := lintSource(t, stamp(t, src), files)
	if got := importFindings(vendored.Findings); len(got) != 0 {
		t.Errorf("a vendored file reported on its own imports: %+v", got)
	}
}

// TestVendored_ImportsStillResolveWhenTheyCan pins that suppressing the errors
// did not switch resolution off: two vendored peers that import each other are
// both present here, and a reference between them still has to resolve to a real
// enum — or be reported when it does not.
func TestVendored_ImportsStillResolveWhenTheyCan(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": "", "docs/payments.modelith.yaml": paymentsModel}
	src := importer([]string{`"./payments.modelith.yaml"`}, "payments.Nonexistent")

	got := importFindings(lintSource(t, stamp(t, src), files).Findings)
	if len(got) != 1 || !strings.Contains(got[0].Message, `names no enum "Nonexistent"`) {
		t.Errorf("want the unresolved item reported, got %+v", got)
	}
}

func TestVendored_DigestMismatch(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": ""}
	stamped := stamp(t, gappy)

	t.Run("an untouched copy verifies", func(t *testing.T) {
		t.Parallel()
		for _, f := range lintSource(t, stamped, files).Findings {
			if strings.Contains(f.Message, "digest") {
				t.Errorf("unexpected digest finding: %s", f.Message)
			}
		}
	})

	t.Run("an edited copy is an error naming both remedies", func(t *testing.T) {
		t.Parallel()
		edited := strings.Replace(stamped, "One car's stay in the garage.", "One car's stay.", 1)
		if edited == stamped {
			t.Fatal("the test did not edit the model")
		}
		res := lintSource(t, edited, files)
		var found *Finding
		for i := range res.Findings {
			if strings.Contains(res.Findings[i].Message, "no longer matches the digest") {
				found = &res.Findings[i]
				break
			}
		}
		if found == nil {
			t.Fatal("no digest finding")
		}
		if found.Severity != SeverityError {
			t.Errorf("digest mismatch is a %s, want an error", found.Severity)
		}
		for _, want := range []string{
			"modelith deps import https://github.com/stacklok/some-repo/blob/main/docs/payments.modelith.yaml",
			"delete the provenance header",
		} {
			if !strings.Contains(found.Message, want) {
				t.Errorf("digest message does not offer %q: %s", want, found.Message)
			}
		}
	})
}

func TestVendored_HeaderProblemsAreReported(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": ""}
	cases := []struct {
		name     string
		header   string
		contains string
	}{
		{"unknown key", "# modelith-surface: sha256:x\n", "unknown provenance key"},
		{"unknown fetch method", "# modelith-fetch: carrier-pigeon\n", `unknown fetch method "carrier-pigeon"`},
		{"missing required key", "# modelith-fetch: git\n", `missing "# modelith-commit"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var found bool
			for _, f := range lintSource(t, tc.header+gappy, files).Findings {
				if strings.Contains(f.Message, tc.contains) {
					if f.Severity != SeverityError {
						t.Errorf("header problem is a %s, want an error", f.Severity)
					}
					found = true
				}
			}
			if !found {
				t.Errorf("no finding containing %q", tc.contains)
			}
		})
	}
}

// TestVendored_MalformedHeaderStillSuppressesCompleteness pins that presence,
// not a clean parse, is what makes a file vendored. A typo in the header would
// otherwise be buried under gaps in a document this repository does not own.
func TestVendored_MalformedHeaderStillSuppressesCompleteness(t *testing.T) {
	t.Parallel()

	files := fakeFiles{".git": ""}
	for _, f := range lintSource(t, "# modelith-surface: sha256:x\n"+gappy, files).Findings {
		if f.Category == CategoryCompleteness {
			t.Errorf("completeness finding survived a malformed header: %s: %s", f.Path, f.Message)
		}
	}
}
