package lint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFiles is a map-backed Files keyed by cleaned, slash-separated path. A
// hand-written fake rather than a mock: it behaves like a filesystem, reporting
// a missing file the way os.ReadFile does and answering containment from its own
// tree. Containment is part of the fake because deciding it on the real disk
// while reading from a map would make these cases depend on where the source
// tree happens to sit, and on whether it is a git checkout at all.
//
// A ".git" key marks a repository, exactly as the entry does on disk. Paths in
// the map are relative, so the whole tree hangs off ".".
type fakeFiles map[string]string

func (f fakeFiles) ReadFile(path string) ([]byte, error) {
	src, ok := f[filepath.ToSlash(filepath.Clean(path))]
	if !ok {
		return nil, fmt.Errorf("open %s: %w", path, fs.ErrNotExist)
	}
	return []byte(src), nil
}

// Resolve cleans the path and stops there: the fake's keys are the whole world
// and hold no symlinks, so there is nothing further to follow.
func (f fakeFiles) Resolve(path string) string { return filepath.Clean(path) }

// ResolutionRoot mirrors OSFiles.ResolutionRoot over the map: the nearest
// ancestor holding a ".git" key, or the model's own directory when there is
// none.
func (f fakeFiles) ResolutionRoot(modelPath string) (string, bool) {
	dir := filepath.Dir(filepath.Clean(modelPath))
	for cur := dir; ; {
		if _, ok := f[filepath.ToSlash(filepath.Join(cur, ".git"))]; ok {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return dir, false
		}
		cur = parent
	}
}

// importerPath is where the model under test lives; its imports resolve
// relative to the "docs" directory.
const importerPath = "docs/garage.modelith.yaml"

const paymentsModel = `kind: DomainModel
version: v1
title: Payments
enums:
  PaymentMethod:
    description: How a bill is settled.
    values:
      - name: card
      - name: transfer
entities:
  Invoice:
    definition: A request for payment.
`

// importer builds a model that lists the given imports and types one attribute
// with the given type. Each entry is written verbatim as a YAML sequence item,
// so a case can use either the bare-path form or the explicit {scope, path} one.
func importer(imports []string, attrType string) string {
	var b strings.Builder
	b.WriteString("kind: DomainModel\nversion: v1\n")
	if len(imports) > 0 {
		b.WriteString("imports:\n")
		for _, imp := range imports {
			fmt.Fprintf(&b, "  - %s\n", imp)
		}
	}
	fmt.Fprintf(&b, `entities:
  Visit:
    definition: One car's stay in the garage.
    attributes:
      - name: settledWith
        type: %s
`, attrType)
	return b.String()
}

type wantFinding struct {
	severity Severity
	category Category
	path     string
	contains string
}

// importFindings narrows a result to the surfaces these tests drive: the
// imports list and the reference sites that resolve against it. Completeness
// gaps in the deliberately thin fixtures are not what is under test.
func importFindings(all []Finding) []Finding {
	var out []Finding
	for _, f := range all {
		switch {
		case strings.HasPrefix(f.Path, "/imports/"),
			strings.HasSuffix(f.Path, "/type"),
			strings.HasSuffix(f.Path, "/entity"),
			strings.HasSuffix(f.Path, "/subtypeOf"):
			out = append(out, f)
		}
	}
	return out
}

func assertFindings(t *testing.T, got []Finding, want []wantFinding) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d finding(s), got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Severity != w.severity || g.Category != w.category || g.Path != w.path {
			t.Errorf("finding %d: got %s/%s at %q, want %s/%s at %q",
				i, g.Severity, g.Category, g.Path, w.severity, w.category, w.path)
		}
		if !strings.Contains(g.Message, w.contains) {
			t.Errorf("finding %d: message %q does not contain %q", i, g.Message, w.contains)
		}
	}
}

func TestImports_Resolution(t *testing.T) {
	t.Parallel()

	files := fakeFiles{
		// The repository marker at the top of the fake tree is what lets an
		// import reach a sibling directory; TestImports_ContainmentUsesTheFileSeam
		// covers the tree without one.
		".git":                             "",
		"docs/payments.modelith.yaml":      paymentsModel,
		"payments/payments.modelith.yaml":  paymentsModel,
		"docs/legacy/pay-v2.modelith.yaml": paymentsModel,
		"docs/not-a-model.yaml":            "kind: SomethingElse\nversion: v1\n",
	}

	const typePath = "/entities/Visit/attributes/0/type"

	cases := []struct {
		name     string
		imports  []string
		attrType string
		want     []wantFinding
	}{
		{
			name:     "peer import resolves",
			imports:  []string{`"./payments.modelith.yaml"`},
			attrType: "payments.PaymentMethod",
		},
		{
			name:     "parent-relative import resolves",
			imports:  []string{`"../payments/payments.modelith.yaml"`},
			attrType: "payments.PaymentMethod",
		},
		{
			name:     "qualified type with no imports at all",
			attrType: "payments.PaymentMethod",
			want: []wantFinding{{
				SeverityError, CategorySemantic, typePath,
				`references the scope "payments", which no import binds`,
			}},
		},
		{
			name:     "qualified type naming an unimported scope",
			imports:  []string{`"./payments.modelith.yaml"`},
			attrType: "shipping.Carrier",
			want: []wantFinding{
				{SeverityError, CategorySemantic, typePath,
					`references the scope "shipping", which no import binds`},
				{SeverityWarning, CategoryCompleteness, "/imports/0", "is never referenced"},
			},
		},
		{
			name:     "qualified type naming no enum in the imported model",
			imports:  []string{`"./payments.modelith.yaml"`},
			attrType: "payments.Nonexistent",
			want: []wantFinding{{
				SeverityError, CategorySemantic, typePath,
				`names no enum "Nonexistent"`,
			}},
		},
		{
			name:     "qualified type resolving to an entity, not an enum",
			imports:  []string{`"./payments.modelith.yaml"`},
			attrType: "payments.Invoice",
			want: []wantFinding{{
				SeverityError, CategorySemantic, typePath,
				`resolves to the entity "Invoice"`,
			}},
		},
		{
			name:     "explicit scope overrides the filename",
			imports:  []string{"{scope: billing, path: ./legacy/pay-v2.modelith.yaml}"},
			attrType: "billing.PaymentMethod",
		},
		{
			name:     "two imports binding the same scope",
			imports:  []string{`"./payments.modelith.yaml"`, `"../payments/payments.modelith.yaml"`},
			attrType: "payments.PaymentMethod",
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/1",
				`binds scope "payments", which import "./payments.modelith.yaml" already binds`,
			}},
		},
		{
			name:     "bare import whose filename is not a usable slug",
			imports:  []string{`"./PayMents.modelith.yaml"`},
			attrType: "string",
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				"which is not a valid slug",
			}},
		},
		{
			name:     "absolute import path",
			imports:  []string{`"/abs/payments.modelith.yaml"`},
			attrType: "string",
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				"is an absolute path",
			}},
		},
		{
			name:     "unreadable import",
			imports:  []string{`"./missing.modelith.yaml"`},
			attrType: "string",
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				"cannot be read",
			}},
		},
		{
			name:     "import that is not a domain model",
			imports:  []string{`"./not-a-model.yaml"`},
			attrType: "string",
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				"is not a domain model",
			}},
		},
		{
			name:     "import nothing references",
			imports:  []string{`"./payments.modelith.yaml"`},
			attrType: "string",
			want: []wantFinding{{
				SeverityWarning, CategoryCompleteness, "/imports/0",
				"is never referenced",
			}},
		},
		{
			name:     "unqualified PascalCase type is still only a warning",
			attrType: "PaymentMethod",
			want: []wantFinding{{
				SeverityWarning, CategorySemantic, typePath,
				"looks like an enum reference",
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Run(importerPath, []byte(importer(tc.imports, tc.attrType)), files)
			if err != nil {
				t.Fatal(err)
			}
			assertFindings(t, importFindings(res.Findings), tc.want)
		})
	}
}

// TestImports_ContainmentUsesTheFileSeam pins that the boundary is judged
// against the filesystem the reads go to, not against whatever tree the tests
// happen to run in. The same model and the same import resolve or are refused
// purely by whether the fake tree carries a repository marker, so neither case
// depends on the source tree being a git checkout — copy it without .git (a
// source tarball, a vendored module, a shallow CI export) and both still hold.
func TestImports_ContainmentUsesTheFileSeam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files fakeFiles
		want  []wantFinding
	}{
		{
			name: "a repository above the model admits a sibling directory",
			files: fakeFiles{
				".git":                            "",
				"payments/payments.modelith.yaml": paymentsModel,
			},
		},
		{
			name:  "no repository confines resolution to the model's directory",
			files: fakeFiles{"payments/payments.modelith.yaml": paymentsModel},
			want: []wantFinding{{
				SeverityError, CategorySemantic, "/imports/0",
				`import "../payments/payments.modelith.yaml" resolves to ` +
					`"payments/payments.modelith.yaml", outside "docs" — this model is in no repository`,
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := importer([]string{`"../payments/payments.modelith.yaml"`}, "payments.PaymentMethod")
			res, err := Run(importerPath, []byte(src), tc.files)
			if err != nil {
				t.Fatal(err)
			}
			assertFindings(t, importFindings(res.Findings), tc.want)
		})
	}
}

// TestImports_MalformedQualifiedType covers a type that contains a dot but is
// not a well-formed "scope.Name". Each shape names the way it is malformed
// rather than passing silently as a primitive.
func TestImports_MalformedQualifiedType(t *testing.T) {
	t.Parallel()

	files := fakeFiles{"docs/payments.modelith.yaml": paymentsModel}
	const typePath = "/entities/Visit/attributes/0/type"

	cases := []struct {
		name     string
		attrType string
		reason   string
		// unusedImport is set where the malformed type names no bound scope, so
		// the import genuinely goes unreferenced.
		unusedImport bool
	}{
		{name: "two dots", attrType: "payments.v2.PaymentMethod", reason: "more than one dot"},
		{name: "leading dot", attrType: ".PaymentMethod", reason: "nothing before the dot", unusedImport: true},
		{name: "trailing dot", attrType: "payments.", reason: "nothing after the dot"},
		{name: "scope is not a slug", attrType: "Payments.PaymentMethod", reason: `the scope "Payments" is not lowercase kebab-case`, unusedImport: true},
		{name: "item is not PascalCase", attrType: "payments.paymentMethod", reason: `the item name "paymentMethod" is not PascalCase`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := []wantFinding{{
				SeverityError, CategorySemantic, typePath,
				fmt.Sprintf("attribute type %q is a malformed cross-model reference (%s)", tc.attrType, tc.reason),
			}}
			if tc.unusedImport {
				want = append(want, wantFinding{
					SeverityWarning, CategoryCompleteness, "/imports/0", "is never referenced",
				})
			}
			src := importer([]string{`"./payments.modelith.yaml"`}, tc.attrType)
			res, err := Run(importerPath, []byte(src), files)
			if err != nil {
				t.Fatal(err)
			}
			assertFindings(t, importFindings(res.Findings), want)
		})
	}
}

// TestImports_PathRejectsControlCharacters checks the lint-side half of the
// injection defence: a path holding a newline or any other control character is
// not a path, and saying so keeps it out of the renderer and out of this
// diagnostic's own line-wise output.
func TestImports_PathRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	src := importer([]string{`{scope: payments, path: "./pay\n## Injected\nments.modelith.yaml"}`}, "payments.PaymentMethod")
	res, err := Run(importerPath, []byte(src), fakeFiles{})
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{{
		SeverityError, CategorySemantic, "/imports/0",
		`import path "./pay\n## Injected\nments.modelith.yaml" contains a control character at byte 5`,
	}})
	for _, f := range res.Findings {
		if strings.ContainsAny(f.Message, "\n\r") {
			t.Errorf("a diagnostic must stay on one line, got %q", f.Message)
		}
	}
}

// TestImports_UnresolvedScopeReportedOnce checks that one missing import is one
// finding however many attributes reference it, and that an import which is
// listed but failed to load does not also make every reference site look like a
// reference to nothing.
func TestImports_UnresolvedScopeReportedOnce(t *testing.T) {
	t.Parallel()

	// Six attributes, all naming the same unbound scope.
	var b strings.Builder
	b.WriteString("kind: DomainModel\nversion: v1\nentities:\n  Visit:\n    definition: One stay.\n    attributes:\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "      - name: a%d\n        type: shipping.Carrier\n", i)
	}
	res, err := Run(importerPath, []byte(b.String()), fakeFiles{})
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{{
		SeverityError, CategorySemantic, "/entities/Visit/attributes/0/type",
		`references the scope "shipping", which no import binds — add the model that defines Carrier to ` +
			"`imports:` (6 attribute types reference this scope; this is the first)",
	}})

	t.Run("an import that failed to load speaks for its own scope", func(t *testing.T) {
		t.Parallel()
		src := importer([]string{`"./typo.modelith.yaml"`}, "typo.PaymentMethod")
		res, err := Run(importerPath, []byte(src), fakeFiles{})
		if err != nil {
			t.Fatal(err)
		}
		assertFindings(t, importFindings(res.Findings), []wantFinding{{
			SeverityError, CategorySemantic, "/imports/0", "cannot be read",
		}})
	})
}

// TestImports_DuplicateScopeSurvivesAnUnreadableBinder checks that the second
// entry's own trouble does not mask the duplicate binding: fixing the path would
// otherwise surface a second, unrelated failure.
func TestImports_DuplicateScopeSurvivesAnUnreadableBinder(t *testing.T) {
	t.Parallel()

	files := fakeFiles{"docs/payments.modelith.yaml": paymentsModel}
	src := importer(
		[]string{`"./payments.modelith.yaml"`, "{scope: payments, path: ./gone.modelith.yaml}"},
		"payments.PaymentMethod",
	)
	res, err := Run(importerPath, []byte(src), files)
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{{
		SeverityError, CategorySemantic, "/imports/1",
		`binds scope "payments", which import "./payments.modelith.yaml" already binds`,
	}})
}

// TestImports_UnsupportedVersionInAnImportedModel checks that an imported file
// is held to the versions this binary understands, not just to its kind.
func TestImports_UnsupportedVersionInAnImportedModel(t *testing.T) {
	t.Parallel()

	files := fakeFiles{
		"docs/payments.modelith.yaml": strings.Replace(paymentsModel, "version: v1", "version: v99", 1),
	}
	src := importer([]string{`"./payments.modelith.yaml"`}, "payments.PaymentMethod")
	res, err := Run(importerPath, []byte(src), files)
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{{
		SeverityError, CategorySemantic, "/imports/0",
		`import "./payments.modelith.yaml" declares schema version "v99", which this modelith does not support: v1`,
	}})
}

// TestImports_SkippedWhenTheSchemaRejectedTheDocument checks that a scope the
// schema already rejected never binds. Advising a fix in terms of a scope that
// cannot exist — "reference it as .Name" — is worse than saying nothing.
func TestImports_SkippedWhenTheSchemaRejectedTheDocument(t *testing.T) {
	t.Parallel()

	files := fakeFiles{"docs/payments.modelith.yaml": paymentsModel}
	for _, scope := range []string{`""`, "Payments", "pay.ments"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			src := importer([]string{"{scope: " + scope + ", path: ./payments.modelith.yaml}"}, "payments.PaymentMethod")
			res, err := Run(importerPath, []byte(src), files)
			if err != nil {
				t.Fatal(err)
			}
			if !findingWithMessage(res.Findings, "does not match pattern") {
				t.Fatalf("expected the schema to reject the scope, got: %+v", res.Findings)
			}
			for _, f := range res.Findings {
				if f.Category != CategoryStructural && strings.HasPrefix(f.Path, "/imports/") {
					t.Errorf("imports must not resolve against a rejected document, got %+v", f)
				}
			}
		})
	}
}

// TestImports_SingleCharacterScope pins the one definition of a scope slug: a
// one-character scope is legal, so "a.modelith.yaml" can be imported bare. The
// schema, the linter and the reference pattern all read it from
// model.ScopeSlug, so they cannot disagree about the minimum length.
func TestImports_SingleCharacterScope(t *testing.T) {
	t.Parallel()

	files := fakeFiles{"docs/a.modelith.yaml": paymentsModel}
	res, err := Run(importerPath, []byte(importer([]string{`"./a.modelith.yaml"`}, "a.PaymentMethod")), files)
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), nil)
}

// TestADR_0012_ImporterBindsScope pins that the scope belongs to the importing
// model: the same file resolves under whatever scope each importer binds it to,
// and a model has no say in — and no field for — what it is called elsewhere.
func TestADR_0012_ImporterBindsScope(t *testing.T) {
	t.Parallel()

	files := fakeFiles{"docs/payments.modelith.yaml": paymentsModel}

	t.Run("same file, different bindings", func(t *testing.T) {
		t.Parallel()
		for _, binding := range []struct{ entry, scope string }{
			{`"./payments.modelith.yaml"`, "payments"},
			{"{scope: money, path: ./payments.modelith.yaml}", "money"},
		} {
			res, err := Run(importerPath, []byte(importer([]string{binding.entry}, binding.scope+".PaymentMethod")), files)
			if err != nil {
				t.Fatal(err)
			}
			assertFindings(t, importFindings(res.Findings), nil)
		}
	})

	t.Run("a model cannot name itself", func(t *testing.T) {
		t.Parallel()
		src := "kind: DomainModel\nversion: v1\nscope: payments\nentities:\n  Visit:\n    definition: One stay.\n"
		res, err := Run(importerPath, []byte(src), files)
		if err != nil {
			t.Fatal(err)
		}
		if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
			t.Fatalf("expected a top-level `scope:` to be rejected, got: %+v", res.Findings)
		}
	})
}

// TestADR_0010_NonTransitiveResolution pins that resolution does not recurse:
// an item defined in a model that an *imported* model imports is unreachable,
// and the file it lives in is never read.
func TestADR_0010_NonTransitiveResolution(t *testing.T) {
	t.Parallel()

	const middle = `kind: DomainModel
version: v1
imports:
  - "./shipping.modelith.yaml"
entities:
  Invoice:
    definition: A request for payment.
`
	const leaf = `kind: DomainModel
version: v1
enums:
  Carrier:
    values:
      - name: ground
`
	read := map[string]int{}
	files := countingFiles{
		fakeFiles: fakeFiles{
			"docs/payments.modelith.yaml": middle,
			"docs/shipping.modelith.yaml": leaf,
		},
		read: read,
	}

	res, err := Run(importerPath, []byte(importer([]string{`"./payments.modelith.yaml"`}, "shipping.Carrier")), files)
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{
		{SeverityError, CategorySemantic, "/entities/Visit/attributes/0/type",
			`references the scope "shipping", which no import binds`},
		{SeverityWarning, CategoryCompleteness, "/imports/0", "is never referenced"},
	})
	if n := read["docs/shipping.modelith.yaml"]; n != 0 {
		t.Errorf("an imported model's own imports must not be read, but shipping was read %d time(s)", n)
	}
}

// countingFiles records how many times each path was read, so a test can assert
// a file was never opened. Everything else about the filesystem it inherits.
type countingFiles struct {
	fakeFiles
	read map[string]int
}

func (c countingFiles) ReadFile(path string) ([]byte, error) {
	c.read[filepath.ToSlash(filepath.Clean(path))]++
	return c.fakeFiles.ReadFile(path)
}

// TestRun_QualifiedEntityReferenceIsDeferred checks the friendly error for a
// cross-model reference in an entity position, and that it replaces — rather
// than joins — the schema's pattern violation and the undefined-entity finding.
func TestRun_QualifiedEntityReferenceIsDeferred(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		path string
	}{
		{
			name: "relationship target",
			path: "/entities/Visit/relationships/0/entity",
			src: `kind: DomainModel
version: v1
entities:
  Visit:
    definition: One car's stay in the garage.
    relationships:
      - entity: payments.Invoice
        cardinality: "n:1"
`,
		},
		{
			name: "subtypeOf",
			path: "/entities/Visit/subtypeOf",
			src: `kind: DomainModel
version: v1
entities:
  Visit:
    definition: One car's stay in the garage.
    subtypeOf: payments.Invoice
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Run(importerPath, []byte(tc.src), fakeFiles{})
			if err != nil {
				t.Fatal(err)
			}
			var at []Finding
			for _, f := range res.Findings {
				if f.Path == tc.path {
					at = append(at, f)
				}
			}
			assertFindings(t, at, []wantFinding{{
				SeverityError, CategoryStructural, tc.path,
				"is a cross-model reference, which is not supported in an entity position",
			}})
			if !res.HasBlocking(false) {
				t.Error("an unsupported cross-model entity reference must block")
			}
			if findingWithMessage(res.Findings, "undefined entity") {
				t.Errorf("one mistake reported twice: %+v", res.Findings)
			}
		})
	}
}

// TestRun_QualifiedEntityReferenceDoesNotGateTheImportsLayer pins that two
// unrelated mistakes are reported in one run. The cross-model entity reference
// used to count as a structural failure, which skipped the imports layer
// entirely: the broken import below stayed invisible until the subtypeOf was
// fixed, and fixing it produced a second, unannounced round of errors.
func TestRun_QualifiedEntityReferenceDoesNotGateTheImportsLayer(t *testing.T) {
	t.Parallel()

	src := `kind: DomainModel
version: v1
imports:
  - "./gone.modelith.yaml"
entities:
  Visit:
    definition: One car's stay in the garage.
    subtypeOf: payments.Payment
`
	res, err := Run(importerPath, []byte(src), fakeFiles{})
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), []wantFinding{
		{SeverityError, CategoryStructural, "/entities/Visit/subtypeOf",
			"is a cross-model reference, which is not supported in an entity position"},
		{SeverityError, CategorySemantic, "/imports/0",
			`import "./gone.modelith.yaml" cannot be read`},
	})
}

// TestRun_NilFilesReadsFromDisk covers the documented default: a nil Files
// resolves imports against the local filesystem.
func TestRun_NilFilesReadsFromDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payments.modelith.yaml"), []byte(paymentsModel), 0o600); err != nil {
		t.Fatal(err)
	}
	src := importer([]string{`"./payments.modelith.yaml"`}, "payments.PaymentMethod")
	res, err := Run(filepath.Join(dir, "garage.modelith.yaml"), []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertFindings(t, importFindings(res.Findings), nil)
}
