package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRenderHTML_OutputOptions(t *testing.T) {
	t.Parallel()
	for _, extension := range []string{"yaml", "yml"} {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			input := writeTemp(t, dir, "m.modelith."+extension, minimalValid)
			defaultPath := filepath.Join(dir, "m.modelith.html")
			stdout, err := run(t, "render", "--format=html", "--stdout", input)
			if err != nil || !strings.HasPrefix(stdout, "<!doctype html>") {
				t.Fatalf("stdout: %v, %q", err, stdout)
			}
			if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
				t.Fatalf("stdout wrote a file: %v", err)
			}
			if _, err := run(t, "render", "--format", "html", "--check", input); err == nil || err.Error() != "cannot read committed output "+defaultPath+": open "+defaultPath+": no such file or directory — regenerate it with `modelith render --format html -- '"+input+"'` and commit the result" {
				t.Fatalf("missing check: %v", err)
			}
			if _, err := run(t, "render", "--format", "html", input); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(defaultPath)
			if err != nil || string(data) != stdout {
				t.Fatalf("default path bytes: %v", err)
			}
			if _, err := run(t, "render", "--format", "html", "--check", input); err != nil {
				t.Fatalf("fresh check: %v", err)
			}
			if err := os.WriteFile(defaultPath, []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := run(t, "render", "--format", "html", "--check", input); err == nil || err.Error() != defaultPath+" is out of date — regenerate it with `modelith render --format html -- '"+input+"'` and commit the result" {
				t.Fatalf("stale check: %v", err)
			}
			out := filepath.Join(dir, "custom.html")
			if _, err := run(t, "render", "--format", "html", "-o", out, input); err != nil {
				t.Fatal(err)
			}
			custom, err := os.ReadFile(out)
			if err != nil || string(custom) != stdout {
				t.Fatalf("custom path bytes: %v", err)
			}
			if _, err := run(t, "render", "--format", "html", "-o", out, "--check", input); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(out, []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			command := "modelith render --format html --out '" + out + "' -- '" + input + "'"
			if _, err := run(t, "render", "--format=html", "--out", out, "--check", input); err == nil || err.Error() != out+" is out of date — regenerate it with `"+command+"` and commit the result" {
				t.Fatalf("custom stale: %v", err)
			}
			if err := os.Remove(out); err != nil {
				t.Fatal(err)
			}
			if _, err := run(t, "render", "--format=html", "--out", out, "--check", input); err == nil || err.Error() != "cannot read committed output "+out+": open "+out+": no such file or directory — regenerate it with `"+command+"` and commit the result" {
				t.Fatalf("custom missing: %v", err)
			}
		})
	}
}

func TestRenderHTML_RecoveryQuotesPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := writeTemp(t, dir, "a ' model.modelith.yaml", minimalValid)
	out := filepath.Join(dir, "out ' file.html")
	if _, err := run(t, "render", "--format=html", "--out", out, "--", input); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := "modelith render --format html --out '" + strings.ReplaceAll(out, "'", "'\"'\"'") + "' -- '" + strings.ReplaceAll(input, "'", "'\"'\"'") + "'"
	if _, err := run(t, "render", "--format=html", "--out", out, "--check", "--", input); err == nil || err.Error() != out+" is out of date — regenerate it with `"+command+"` and commit the result" {
		t.Fatalf("quoted recovery: %v", err)
	}
}

func TestRenderHTML_FlagValidation(t *testing.T) {
	t.Parallel()
	input := writeTemp(t, t.TempDir(), "m.modelith.yaml", minimalValid)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"render", "--format=pdf", input}, `invalid render format "pdf": must be markdown or html`},
		{[]string{"render", "--format=html", "--stdout", "--check", input}, "if any flags in the group [stdout check] are set none of the others can be; [check stdout] were all set"},
		{[]string{"render", "--format=html", "--stdout", "-o", "out.html", input}, "if any flags in the group [stdout out] are set none of the others can be; [out stdout] were all set"},
	} {
		if _, err := run(t, tc.args...); err == nil || err.Error() != tc.want {
			t.Fatalf("flags %v: got %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestRenderHTML_VendoredCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := writeTemp(t, dir, "m.modelith.yaml", vendorHeader(minimalValid)+minimalValid)
	out, err := run(t, "render", "--format=html", "--check", input)
	if err != nil || !strings.Contains(out, "vendored copy") {
		t.Fatalf("vendored skip: %v %s", err, out)
	}
	if _, err := run(t, "render", "--format=html", input); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "render", "--format=html", "--check", input); err != nil {
		t.Fatal(err)
	}
}

func TestRenderHTML_FormatCompletion(t *testing.T) {
	t.Parallel()
	fn, ok := renderCmd().GetFlagCompletionFunc("format")
	if !ok {
		t.Fatal("no format completion")
	}
	values, directive := fn(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp || strings.Join(values, ",") != "markdown,html" {
		t.Fatalf("completion = %v, %v", values, directive)
	}
}
