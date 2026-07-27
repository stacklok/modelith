// Command modelith is the Stacklok domain-model tool: it lints domain-model YAML
// files and renders them to Markdown (with embedded Mermaid).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stacklok/modelith/internal/deps"
	"github.com/stacklok/modelith/internal/lint"
	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/provenance"
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
	root.AddCommand(lintCmd(), renderCmd(), schemaCmd(), depsCmd())
	return root
}

// ---- deps ----

func depsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Manage models vendored from other repositories",
		Long: strings.TrimSpace(`
Manage models vendored from other repositories.

A vendored model is a copy of a model whose home is elsewhere, committed here
and marked with a provenance header. Commands in this group are the only ones
that use the network; lint and render never do.`),
	}
	cmd.AddCommand(depsImportCmd())
	return cmd
}

func depsImportCmd() *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "import <url> [dir]",
		Short: "Vendor a model from another repository",
		Long: strings.TrimSpace(`
Vendor a model from another repository into this one.

<url> is the address of the file as it appears in a browser on github.com. The
copy is written into [dir] (the working directory by default) with a provenance
header recording where it came from, and is verified against that header by
every later lint.

Fetching is delegated to the gh CLI, which must be installed and authenticated.
The imports list of the model that will reference this copy is yours to edit;
this command prints the entry to add.`),
		Example: strings.TrimSpace(`
  modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml
  modelith deps import https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml docs/
  modelith deps import --ref v2.1.0 https://github.com/acme/billing/blob/main/docs/payments.modelith.yaml`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 2 {
				dir = args[1]
			}
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory — the destination is a directory, and the filename comes from the origin", dir)
			}

			res, err := deps.Import(cmd.Context(), deps.Options{
				URL: args[0],
				Dir: dir,
				Ref: ref,
				Now: time.Now(),
			})
			if err != nil {
				return err
			}

			printImportResult(cmd.OutOrStdout(), cmd.ErrOrStderr(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "ref to fetch, overriding the one in the URL (a tag pins the copy)")
	return cmd
}

// printImportResult reports a completed import: what was written, the entry to
// paste, whether the fetched model reaches for models of its own, and the trust
// warning ADR-0014 requires.
//
// The warning prints and does not prompt. An agent usually drives this command,
// so a prompt is either auto-answered theatre or a wedged non-interactive run.
// The fetched file is inert until it is named in an imports list, and this
// command deliberately does not do that — the manual step is the real gate.
func printImportResult(out, errOut io.Writer, res *deps.Result) {
	verb := "wrote"
	if res.Replaced {
		verb = "replaced"
	}
	name := filepath.Base(res.Path)
	fmt.Fprintf(out, "%s %s at %s\n", verb, res.Path, res.Header.Commit)
	fmt.Fprintf(out, "\nAdd it to the model that references it, as a path relative to that model:\n\n"+
		"  imports:\n    - ./%s\n", name)
	if n := len(res.TheirImports); n > 0 {
		declares := fmt.Sprintf("declares %d imports of its own", n)
		if n == 1 {
			declares = "declares an import of its own"
		}
		fmt.Fprintf(out, "\nNote: %s %s (%s).\n"+
			"modelith vendors one file, not a dependency tree, and resolution is not\n"+
			"transitive — if you need items from those models, import them directly.\n",
			name, declares, strings.Join(res.TheirImports, ", "))
	}
	fmt.Fprint(errOut, "\nWarning: a vendored model is untrusted content that will be rendered\n"+
		"into your published Markdown. Only vendor from sources you trust.\n")
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
				res, err := lint.Run(path, data, lint.OSFiles{})
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
			// A vendored model's rendered form belongs to its home repository,
			// so it arrives with no committed .md and no obligation to carry
			// one. --check runs over globs, and demanding one here would make
			// this repository regenerate somebody else's document every time
			// their model moved (ADR-0015). Naming the file to render it still
			// renders it; that is how a deep link into a vendored model's .md
			// gets something to point at.
			if check && provenance.Present(data) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is a vendored copy — skipped\n", in)
				return nil
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

			sourceDir, err := filepath.Abs(filepath.Dir(in))
			if err != nil {
				return fmt.Errorf("resolving %s: %w", in, err)
			}

			if stdout {
				// There is no output file to relativize import links against, so
				// they stay relative to the source — the same links a default,
				// beside-the-source render would produce.
				rendered := markdown.Render(m, sourceDir, sourceDir)
				_, err := fmt.Fprint(cmd.OutOrStdout(), rendered)
				return err
			}

			target := out
			if target == "" {
				target = defaultOut(in)
			}
			outDir, err := filepath.Abs(filepath.Dir(target))
			if err != nil {
				return fmt.Errorf("resolving %s: %w", target, err)
			}
			rendered := markdown.Render(m, sourceDir, outDir)

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

// defaultOut is where render writes when no -o is given: a model's .md beside
// its .yaml source. It is the rendered location an import link assumes for the
// model it names (see importLinkTarget in internal/render/markdown) — a link
// resolves only when the imported model was, in fact, rendered here.
func defaultOut(in string) string { return model.RenderedPath(in) }

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
