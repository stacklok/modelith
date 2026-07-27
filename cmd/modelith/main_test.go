package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
