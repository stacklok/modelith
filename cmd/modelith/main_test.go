package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/deps"
	"github.com/stacklok/modelith/internal/provenance"
)

// run executes the CLI with args, capturing stdout+stderr, and returns the
// output and the error main() would turn into an exit code.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := rootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

const minimalValid = `kind: DomainModel
version: v1
entities:
  Thing:
    definition: A thing that exists in the model.
`

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLintCleanFileSucceeds(t *testing.T) {
	f := writeTemp(t, t.TempDir(), "ok.modelith.yaml", minimalValid)
	out, err := run(t, "lint", f)
	if err != nil {
		t.Fatalf("expected clean lint, got error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "0 error(s)") {
		t.Fatalf("expected 0 errors in output, got:\n%s", out)
	}
}

func TestLintBlockingReturnsErrBlocking(t *testing.T) {
	const bad = `kind: DomainModel
version: v1
entities:
  Thing:
    definition: A thing pointing at a nonexistent entity.
    relationships:
      - entity: Nonexistent
        cardinality: "1:1"
`
	f := writeTemp(t, t.TempDir(), "bad.modelith.yaml", bad)
	_, err := run(t, "lint", f)
	if !errors.Is(err, errBlocking) {
		t.Fatalf("expected errBlocking (non-zero exit), got %v", err)
	}
}

func TestLintFormatJSONEmitsFindings(t *testing.T) {
	const bad = `kind: DomainModel
version: v1
entities:
  Thing:
    definition: A thing pointing at a nonexistent entity.
    relationships:
      - entity: Nonexistent
        cardinality: "1:1"
`
	f := writeTemp(t, t.TempDir(), "bad.modelith.yaml", bad)
	// Blocking findings still return errBlocking; we only assert the JSON shape.
	out, _ := run(t, "lint", "--format", "json", f)
	var payload struct {
		Files []struct {
			File     string `json:"file"`
			Findings []struct {
				Severity string `json:"severity"`
				Category string `json:"category"`
				Message  string `json:"message"`
			} `json:"findings"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("--format json did not emit valid JSON: %v\noutput:\n%s", err, out)
	}
	if len(payload.Files) != 1 || len(payload.Files[0].Findings) == 0 {
		t.Fatalf("expected one file with findings, got: %s", out)
	}
}

func TestLintInvalidFlagValues(t *testing.T) {
	f := writeTemp(t, t.TempDir(), "ok.modelith.yaml", minimalValid)
	for _, tc := range []struct {
		name, flag, val, want string
	}{
		{"completeness", "--completeness", "loose", "--completeness must be"},
		{"format", "--format", "xml", "--format must be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, "lint", tc.flag, tc.val, f)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got: %v", tc.want, err)
			}
		})
	}
}

func TestLintMissingFileErrors(t *testing.T) {
	_, err := run(t, "lint", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestRenderWritesFileBesideSource(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", minimalValid)
	out, err := run(t, "render", yamlPath)
	if err != nil {
		t.Fatalf("render failed: %v\noutput:\n%s", err, out)
	}
	mdPath := filepath.Join(dir, "m.modelith.md")
	rendered, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("expected %s to be written: %v", mdPath, err)
	}
	if len(rendered) == 0 {
		t.Fatal("rendered output is empty")
	}
	// --check against the freshly written file must now pass.
	if _, err := run(t, "render", "--check", yamlPath); err != nil {
		t.Fatalf("--check on freshly rendered file should pass, got: %v", err)
	}
}

func TestRenderOutFlagWritesToPath(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", minimalValid)
	target := filepath.Join(dir, "custom.md")
	if _, err := run(t, "render", "-o", target, yamlPath); err != nil {
		t.Fatalf("render -o failed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected %s to be written: %v", target, err)
	}
}

func TestRenderStdoutDoesNotWriteFile(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", minimalValid)
	out, err := run(t, "render", "--stdout", yamlPath)
	if err != nil {
		t.Fatalf("render --stdout failed: %v", err)
	}
	if !strings.Contains(out, "Thing") {
		t.Fatalf("expected rendered Markdown on stdout, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.modelith.md")); !os.IsNotExist(err) {
		t.Fatal("--stdout should not write a file beside the source")
	}
}

// TestRenderOutFlagRelativizesImportLinks pins the R2-1 fix at the CLI level:
// `render -o <dir>/x.md` used to emit import links relative to the source
// directory, so a link built for `-o` landed at a path that didn't exist next
// to the file `-o` actually wrote. The output directory here (a fresh temp
// dir) shares no ancestor with the source's temp dir short of the OS temp
// root, so a fix that only handles a common-prefix case would still fail
// this.
func TestRenderOutFlagRelativizesImportLinks(t *testing.T) {
	srcDir := t.TempDir()
	const payments = `kind: DomainModel
version: v1
entities:
  Payment:
    definition: A payment.
enums:
  PaymentMethod:
    values:
      - {name: card, definition: A card payment.}
`
	paymentsYAML := writeTemp(t, srcDir, "payments.modelith.yaml", payments)
	if _, err := run(t, "render", paymentsYAML); err != nil {
		t.Fatalf("rendering the imported model failed: %v", err)
	}

	const main = `kind: DomainModel
version: v1
imports:
  - ./payments.modelith.yaml
entities:
  Ticket:
    definition: A parking ticket.
    attributes:
      - {name: paidWith, type: payments.PaymentMethod, description: how the fee was paid}
`
	mainYAML := writeTemp(t, srcDir, "main.modelith.yaml", main)

	outDir := t.TempDir() // a distinct temp dir: no shared ancestor but the OS temp root.
	target := filepath.Join(outDir, "rendered.md")
	if _, err := run(t, "render", "-o", target, mainYAML); err != nil {
		t.Fatalf("render -o failed: %v", err)
	}
	rendered, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected %s to be written: %v", target, err)
	}

	link := importLinkFromMarkdown(t, string(rendered))
	resolved := filepath.Join(filepath.Dir(target), link)
	if _, err := os.Stat(resolved); err != nil {
		t.Fatalf("import link %q resolved to %s, which does not exist: %v\nrendered output:\n%s", link, resolved, err, rendered)
	}
}

// importLinkFromMarkdown extracts the destination of the single "[rendered](...)"
// link an Imports section emits, failing the test if it can't find exactly one.
func importLinkFromMarkdown(t *testing.T, md string) string {
	t.Helper()
	const marker = "([rendered]("
	start := strings.Index(md, marker)
	if start < 0 {
		t.Fatalf("no import link found in:\n%s", md)
	}
	start += len(marker)
	end := strings.IndexByte(md[start:], ')')
	if end < 0 {
		t.Fatalf("unterminated import link in:\n%s", md)
	}
	return md[start : start+end]
}

func TestRenderInvalidFileGivesFriendlyError(t *testing.T) {
	// Missing the required `definition`, so structural validation fails.
	const invalid = `kind: DomainModel
version: v1
entities:
  Thing: {}
`
	f := writeTemp(t, t.TempDir(), "invalid.modelith.yaml", invalid)
	_, err := run(t, "render", f)
	if err == nil {
		t.Fatal("expected a structural error rendering an invalid file, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid domain model") {
		t.Fatalf("expected the friendly 'run modelith lint' error, got: %v", err)
	}
}

func TestRenderCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", minimalValid)
	writeTemp(t, dir, "m.modelith.md", "stale content\n")
	_, err := run(t, "render", "--check", yamlPath)
	if err == nil {
		t.Fatal("expected a drift error, got nil")
	}
	if !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("expected an out-of-date error, got: %v", err)
	}
}

// vendorHeader stamps content the way `deps import` does, so a test fixture is
// a vendored copy that verifies against its own digest.
func vendorHeader(content string) string {
	return "# modelith-vendored: " + provenance.Banner + "\n" +
		"# modelith-fetch: git\n" +
		"# modelith-origin: https://github.com/stacklok/some-repo\n" +
		"# modelith-path: docs/m.modelith.yaml\n" +
		"# modelith-ref: main\n" +
		"# modelith-commit: 4f2c1e9\n" +
		"# modelith-imported: 2026-07-27\n" +
		"# modelith-digest: " + provenance.Digest([]byte(content)) + "\n"
}

// TestRenderCheckSkipsAVendoredCopy covers the half of ADR-0015 that lives in
// the CLI: a vendored model has no committed .md here, and --check runs over
// globs, so demanding one would fail a build over somebody else's document.
// Rendering it by name still works — that is what a deep link points at.
func TestRenderCheckSkipsAVendoredCopy(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", vendorHeader(minimalValid)+minimalValid)

	// No .md exists at all, which is the state a fresh `deps import` leaves.
	out, err := run(t, "render", "--check", yamlPath)
	if err != nil {
		t.Fatalf("--check failed on a vendored copy: %v (%s)", err, out)
	}
	if !strings.Contains(out, "vendored copy") {
		t.Errorf("output does not say why it was skipped: %q", out)
	}

	if _, err := os.Stat(filepath.Join(dir, "m.modelith.md")); !os.IsNotExist(err) {
		t.Error("--check wrote a file")
	}

	if _, err := run(t, "render", yamlPath); err != nil {
		t.Fatalf("rendering a vendored model by name failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.modelith.md")); err != nil {
		t.Errorf("rendering by name wrote nothing: %v", err)
	}
}

// TestRenderCheckChecksACommittedVendoredMarkdown pins the other side of the
// skip: once this repository has committed a vendored model's .md — the way a
// deep link into it gets a target — that file goes stale like any other, and
// nothing else would say so. Skipping it unconditionally left a refreshed copy
// with a rendered document describing the version before it.
func TestRenderCheckChecksACommittedVendoredMarkdown(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", vendorHeader(minimalValid)+minimalValid)

	if _, err := run(t, "render", yamlPath); err != nil {
		t.Fatalf("rendering a vendored model by name failed: %v", err)
	}
	out, err := run(t, "render", "--check", yamlPath)
	if err != nil {
		t.Fatalf("--check failed on a freshly rendered copy: %v (%s)", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("output does not report the check: %q", out)
	}

	writeTemp(t, dir, "m.modelith.md", "stale content\n")
	if _, err := run(t, "render", "--check", yamlPath); err == nil {
		t.Fatal("a stale committed .md for a vendored copy passed --check")
	} else if !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("expected an out-of-date error, got: %v", err)
	}
}

// TestRenderCheckRejectsAnOutPathUnderNoDirectory pins that the vendored skip
// does not swallow a misconfigured -o. A target under a directory that does not
// exist stats as ErrNotExist exactly like an uncommitted one, so reading it as
// "nothing committed yet" let a typo pass this gate on a vendored model while
// the same typo on a model this repository owns failed.
func TestRenderCheckRejectsAnOutPathUnderNoDirectory(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "nosuchdir", "out.md")

	for _, tc := range []struct{ name, src string }{
		{"a vendored copy", vendorHeader(minimalValid) + minimalValid},
		{"a model this repository owns", minimalValid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			yamlPath := writeTemp(t, dir, "m.modelith.yaml", tc.src)
			out, err := run(t, "render", "--check", "-o", bad, yamlPath)
			if err == nil {
				t.Fatalf("a target under a missing directory passed --check: %s", out)
			}
		})
	}
}

// TestRenderCheckStaysQuietOnAVendoredCopyItCannotRender pins that --check does
// not turn this repository's build red over a defect in a document it does not
// own. A copy fetched from a repository on a newer modelith can be unreadable
// here; `modelith lint` reports that, once, and --check has nothing to add
// because there is no .md this repository could regenerate.
func TestRenderCheckStaysQuietOnAVendoredCopyItCannotRender(t *testing.T) {
	dir := t.TempDir()
	future := strings.Replace(minimalValid, "version: v1", "version: v99", 1)
	if future == minimalValid {
		t.Fatal("fixture did not declare a version to replace")
	}
	yamlPath := writeTemp(t, dir, "m.modelith.yaml", vendorHeader(future)+future)
	writeTemp(t, dir, "m.modelith.md", "a rendered form from a modelith that understood it\n")

	out, err := run(t, "render", "--check", yamlPath)
	if err != nil {
		t.Fatalf("--check failed on an unrenderable vendored copy: %v (%s)", err, out)
	}
	if !strings.Contains(out, "cannot render") {
		t.Errorf("output does not say why it was skipped: %q", out)
	}

	// The same file, owned rather than vendored, is still this repository's
	// problem and still fails.
	ownPath := writeTemp(t, dir, "own.modelith.yaml", future)
	writeTemp(t, dir, "own.modelith.md", "whatever\n")
	if _, err := run(t, "render", "--check", ownPath); err == nil {
		t.Fatal("an unrenderable model this repository owns passed --check")
	}
}

// TestDepsImportOutput covers what a completed import tells the user. All three
// parts are load-bearing: the snippet is the second, manual step this command
// deliberately leaves to the user, the note is the whole answer to
// non-transitive vendoring, and the warning is what ADR-0014 requires in place
// of hardening the renderer further.
func TestDepsImportOutput(t *testing.T) {
	res := &deps.Result{
		Path:   filepath.Join("docs", "payments.modelith.yaml"),
		Header: &provenance.Header{Commit: "4f2c1e9"},
	}

	t.Run("a leaf model", func(t *testing.T) {
		var out, errOut bytes.Buffer
		printImportResult(&out, &errOut, res)
		// The entry names where the copy actually landed. Printing the bare
		// basename told a user who passed a directory to paste a path that
		// resolves to nothing.
		for _, want := range []string{"wrote docs/payments.modelith.yaml at 4f2c1e9", "imports:", "- ./docs/payments.modelith.yaml"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out.String())
			}
		}
		if strings.Contains(out.String(), "dependency tree") {
			t.Error("a leaf model got the transitive-imports note")
		}
		if !strings.Contains(errOut.String(), "Only vendor from sources you trust") {
			t.Errorf("the trust warning is missing from stderr:\n%s", errOut.String())
		}
	})

	t.Run("a model with imports of its own", func(t *testing.T) {
		var out, errOut bytes.Buffer
		chained := *res
		chained.TheirImports = []string{"./ledger.modelith.yaml"}
		printImportResult(&out, &errOut, &chained)
		for _, want := range []string{
			"declares an import of its own (./ledger.modelith.yaml)",
			"resolution is not",
			"import them directly",
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out.String())
			}
		}
	})

	t.Run("a copy in the working directory", func(t *testing.T) {
		var out, errOut bytes.Buffer
		here := *res
		here.Path = "payments.modelith.yaml"
		printImportResult(&out, &errOut, &here)
		if !strings.Contains(out.String(), "- ./payments.modelith.yaml") {
			t.Errorf("stdout does not contain the entry:\n%s", out.String())
		}
	})

	t.Run("a replaced copy says so", func(t *testing.T) {
		var out, errOut bytes.Buffer
		replaced := *res
		replaced.Replaced = true
		printImportResult(&out, &errOut, &replaced)
		if !strings.HasPrefix(out.String(), "replaced ") {
			t.Errorf("stdout does not report the replacement:\n%s", out.String())
		}
	})
}

// TestDepsImportRejectsBadArguments covers the paths that fail before any fetch
// is attempted, so they need no network and no gh.
func TestDepsImportRejectsBadArguments(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "not-a-dir", "x")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a host modelith cannot fetch from",
			args: []string{"deps", "import", "https://gitlab.com/acme/billing/-/blob/main/m.modelith.yaml", dir},
			want: "github.com/stacklok/modelith/issues",
		},
		{
			name: "a URL that names no file",
			args: []string{"deps", "import", "https://github.com/acme/billing", dir},
			want: "not a GitHub file URL",
		},
		{
			name: "a destination that is a file",
			args: []string{"deps", "import", "https://github.com/acme/billing/blob/main/m.modelith.yaml", file},
			want: "is not a directory",
		},
		{
			name: "a destination that does not exist",
			args: []string{"deps", "import", "https://github.com/acme/billing/blob/main/m.modelith.yaml", filepath.Join(dir, "nope")},
			want: "no such file or directory",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSchemaOutputsValidJSON(t *testing.T) {
	out, err := run(t, "schema")
	if err != nil {
		t.Fatalf("schema command errored: %v", err)
	}
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("schema output is not valid JSON: %v", err)
	}
}
