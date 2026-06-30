// Command modelith is the Stacklok domain-model tool: it lints domain-model YAML
// files and renders them to Markdown (with embedded Mermaid).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stacklok/modelith/internal/lint"
	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/render/markdown"
	"github.com/stacklok/modelith/internal/schema"
)

// version is set at build time via -ldflags "-X main.version=..." by
// goreleaser. When unset (e.g. `go install ...@version`), buildVersion derives
// it from the embedded build info.
var version = ""

// buildVersion resolves the version string, in precedence order: an explicit
// ldflags override, the module version embedded by `go install module@version`,
// then VCS info for a local `go build`/`go install ./...` checkout.
func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 12 {
				rev = s.Value[:12]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return "devel-" + rev + dirty
	}
	return "dev"
}

// errBlocking signals that `lint` found blocking findings. RunE returns it
// rather than calling os.Exit, so deferred cleanup runs and the blocking path
// is testable; main() turns it into a non-zero exit without re-printing it (the
// findings are already on stdout).
var errBlocking = errors.New("blocking findings")

func main() {
	if err := rootCmd().Execute(); err != nil {
		if !errors.Is(err, errBlocking) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "modelith",
		Short:         "Author, validate, and render Stacklok domain models",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildVersion(),
	}
	root.AddCommand(lintCmd(), renderCmd(), schemaCmd())
	return root
}

// ---- lint ----

func lintCmd() *cobra.Command {
	var (
		completeness string
		format       string
	)
	cmd := &cobra.Command{
		Use:   "lint <file>...",
		Short: "Validate domain-model files (structural, semantic, completeness)",
		Example: strings.TrimSpace(`
  modelith lint model.modelith.yaml
  modelith lint services/*.modelith.yaml            # multiple files / globs
  modelith lint --completeness error model.modelith.yaml
  modelith lint --format json model.modelith.yaml   # machine-readable for CI`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch completeness {
			case "warn", "error":
			default:
				return fmt.Errorf("--completeness must be warn or error, got %q", completeness)
			}
			switch format {
			case "text", "json":
			default:
				return fmt.Errorf("--format must be text or json, got %q", format)
			}
			completenessAsError := completeness == "error"

			type fileResult struct {
				File     string         `json:"file"`
				Findings []lint.Finding `json:"findings"`
			}
			var all []fileResult
			blocking := false

			for _, path := range args {
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				res, err := lint.Run(data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				all = append(all, fileResult{File: path, Findings: res.Findings})
				if res.HasBlocking(completenessAsError) {
					blocking = true
				}
			}

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(map[string]any{"files": all}); err != nil {
					return err
				}
			} else {
				out := cmd.OutOrStdout()
				var errs, warns int
				for _, fr := range all {
					fmt.Fprintf(out, "%s:\n", fr.File)
					if len(fr.Findings) == 0 {
						fmt.Fprintln(out, "  ok")
					}
					for _, f := range fr.Findings {
						if f.Severity == lint.SeverityError {
							errs++
						} else {
							warns++
						}
						loc := f.Path
						if loc == "" {
							loc = "(root)"
						}
						fmt.Fprintf(out, "  %-7s [%s] %s: %s\n", f.Severity, f.Category, loc, f.Message)
					}
				}
				fmt.Fprintf(out, "\n%d error(s), %d warning(s)\n", errs, warns)
			}

			if blocking {
				return errBlocking
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&completeness, "completeness", "warn", "treat completeness gaps as warn or error")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	_ = cmd.RegisterFlagCompletionFunc("completeness",
		cobra.FixedCompletions([]string{"warn", "error"}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("format",
		cobra.FixedCompletions([]string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

// ---- render ----

func renderCmd() *cobra.Command {
	var (
		out    string
		stdout bool
		check  bool
	)
	cmd := &cobra.Command{
		Use:   "render <file>",
		Short: "Render a domain-model file to Markdown (with embedded Mermaid)",
		Example: strings.TrimSpace(`
  modelith render model.modelith.yaml            # write model.modelith.md beside the source
  modelith render -o out.md model.modelith.yaml  # write to a specific path
  modelith render --stdout model.modelith.yaml   # write to stdout instead of a file
  modelith render --check model.modelith.yaml    # CI gate: fail if the committed .md is stale`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := args[0]
			data, err := os.ReadFile(in)
			if err != nil {
				return fmt.Errorf("%s: %w", in, err)
			}
			// Validate against the schema first so a malformed file fails with a
			// friendly, located error rather than the raw strict-YAML parse error.
			if findings := lint.Structural(data); len(findings) > 0 {
				var b strings.Builder
				fmt.Fprintf(&b, "%s is not a valid domain model — run `modelith lint %s` for details:", in, in)
				for _, f := range findings {
					loc := f.Path
					if loc == "" {
						loc = "(root)"
					}
					fmt.Fprintf(&b, "\n  %s: %s", loc, f.Message)
				}
				return errors.New(b.String())
			}
			m, err := model.Parse(data)
			if err != nil {
				return err
			}
			rendered := markdown.Render(m)

			if stdout {
				_, err := fmt.Fprint(cmd.OutOrStdout(), rendered)
				return err
			}

			target := out
			if target == "" {
				target = defaultOut(in)
			}

			if check {
				existing, err := os.ReadFile(target)
				if err != nil {
					return fmt.Errorf("cannot read committed output %s: %w — regenerate it with `modelith render %s` and commit the result", target, err, in)
				}
				if string(existing) != rendered {
					return fmt.Errorf("%s is out of date — regenerate it with `modelith render %s` and commit the result", target, in)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s is up to date\n", target)
				return nil
			}

			if err := os.WriteFile(target, []byte(rendered), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", target)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output path (default: input with .md extension)")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "write to stdout instead of a file")
	cmd.Flags().BoolVar(&check, "check", false, "verify the committed output is up to date; non-zero exit on drift")
	// --stdout has no output file, so it conflicts with both --out and --check.
	cmd.MarkFlagsMutuallyExclusive("stdout", "out")
	cmd.MarkFlagsMutuallyExclusive("stdout", "check")
	return cmd
}

func defaultOut(in string) string {
	ext := filepath.Ext(in)
	if ext == ".yaml" || ext == ".yml" {
		return strings.TrimSuffix(in, ext) + ".md"
	}
	return in + ".md"
}

// ---- schema ----

func schemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "Print the canonical JSON Schema",
		Example: "  modelith schema > modelith.schema.json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write(schema.JSON())
			return err
		},
	}
}
