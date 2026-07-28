// Command modelith is the Stacklok domain-model tool: it lints domain-model YAML
// files and renders them to Markdown (with embedded Mermaid).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	cmd.AddCommand(depsImportCmd(), depsCheckCmd(), depsUpdateCmd())
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

			// A failure to read the working directory only costs the entry
			// its relative form, so it is not worth failing a completed
			// import over.
			wd, _ := os.Getwd()
			printImportResult(cmd.OutOrStdout(), cmd.ErrOrStderr(), res, wd)
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
func printImportResult(out, errOut io.Writer, res *deps.Result, wd string) {
	verb := "wrote"
	if res.Replaced {
		verb = "replaced"
	}
	name := filepath.Base(res.Path)
	fmt.Fprintf(out, "%s %s at %s\n", verb, res.Path, res.Header.Commit)
	// The entry is printed relative to the working directory, because that is
	// the only thing this command knows: an import path is relative to the model
	// that declares it, and which model that will be is the user's to decide —
	// the same reason the imports list is not edited here.
	fmt.Fprintf(out, "\nAdd it to the model that references it, as a path relative to that model "+
		"(this one is relative to the current directory):\n\n"+
		"  imports:\n    - %s\n", importEntry(res.Path, wd))
	printTransitiveNote(out, name, res.TheirImports, "declares")
	printTrustWarning(errOut)
}

// printTransitiveNote says that a model reaches for models of its own and that
// modelith did not follow them. verb differs between a first import, where the
// imports are simply what the model has, and an update, where they are what it
// gained since the copy on disk was taken.
func printTransitiveNote(out io.Writer, name string, imports []string, verb string) {
	n := len(imports)
	if n == 0 {
		return
	}
	declares := fmt.Sprintf("%s %d imports of its own", verb, n)
	if n == 1 {
		declares = fmt.Sprintf("%s an import of its own", verb)
	}
	fmt.Fprintf(out, "\nNote: %s %s (%s).\n"+
		"modelith vendors one file, not a dependency tree, and resolution is not\n"+
		"transitive — if you need items from those models, import them directly.\n",
		name, declares, strings.Join(imports, ", "))
}

// printTrustWarning is ADR-0014's warning, printed whenever third-party content
// lands in the repository. That an origin was trusted at import time says
// nothing about the commit that landed since, so an update repeats it.
func printTrustWarning(errOut io.Writer) {
	fmt.Fprint(errOut, "\nWarning: a vendored model is untrusted content that will be rendered\n"+
		"into your published Markdown. Only vendor from sources you trust.\n")
}

func depsCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <file>...",
		Short: "Report which vendored copies have fallen behind their origin",
		Long: strings.TrimSpace(`
Report which vendored copies have fallen behind their origin.

A copy is stale when its origin now serves different content, compared against
the digest the copy's own header records. A file with no provenance header is
skipped, so the glob you already pass to lint works here unchanged; a closing
line says how many were skipped. Exits non-zero when any copy is stale.

This command answers what the origin has done. Whether a copy here has been
hand-edited is a different question, and modelith lint answers it offline.

A copy pinned to a tag is reported as up to date for as long as that tag points
where it did; modelith does not look for newer releases. Every line names the
ref it was checked against.

Fetching is delegated to the gh CLI, which must be installed and authenticated.
Nothing is written.`),
		Example: strings.TrimSpace(`
  modelith deps check docs/payments.modelith.yaml
  modelith deps check docs/*.modelith.yaml`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := deps.Check(cmd.Context(), deps.CheckOptions{Paths: args})
			// A run that got nowhere has nothing to summarise, and "checked 0
			// vendored copies" above the reason would read as the outcome.
			blocking := len(reports) > 0 && printCheckReports(cmd.OutOrStdout(), cmd.ErrOrStderr(), reports)
			if err != nil {
				return err
			}
			if blocking {
				return errBlocking
			}
			return nil
		},
	}
	return cmd
}

func depsUpdateCmd() *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "update <file>...",
		Short: "Bring vendored copies forward to what their origins serve",
		Long: strings.TrimSpace(`
Bring vendored copies forward to what their origins serve.

A copy whose origin has not moved is left byte for byte alone, so running this
over a glob produces a diff only where something actually changed. A copy that
was hand-edited is not holding what its origin serves, so it is rewritten and
the edits are discarded — make the change at the origin instead.

--ref re-pins one copy to a different tag or branch, which is how a pinned copy
moves; a copy tracking a branch moves on a bare update. It applies to exactly
one file, because one ref names a different version in every other repository.

This command writes the copies and nothing else. It does not edit any model's
imports list, and it does not lint: an item a copy used to define may have been
renamed or removed upstream, and modelith lint is what reports that from the
importing model's seat.

Fetching is delegated to the gh CLI, which must be installed and authenticated.`),
		Example: strings.TrimSpace(`
  modelith deps update docs/payments.modelith.yaml
  modelith deps update docs/*.modelith.yaml
  modelith deps update --ref v2.2.0 docs/payments.modelith.yaml`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := deps.Update(cmd.Context(), deps.UpdateOptions{
				Paths: args,
				Ref:   ref,
				Now:   time.Now(),
			})
			// See depsCheckCmd: nothing reached means nothing to summarise.
			blocking := len(reports) > 0 && printUpdateReports(cmd.OutOrStdout(), cmd.ErrOrStderr(), reports)
			if err != nil {
				return err
			}
			if blocking {
				return errBlocking
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "re-pin the copy to this ref (one file only)")
	return cmd
}

// printCheckReports reports a check run and says whether it should exit
// non-zero: a stale copy or a copy that could not be reached both qualify.
func printCheckReports(out, errOut io.Writer, reports []deps.Report) bool {
	stale := 0
	for i := range reports {
		r := &reports[i]
		switch {
		case r.Skipped:
		case r.Err != nil:
			fmt.Fprintf(errOut, "%s: %v\n", r.Path, r.Err)
		case r.State == nil:
			// A file the run abandoned rather than judged. survey does not hand
			// one back, and this keeps a future one that does from crashing a
			// command whose whole job is to report calmly on other people's
			// files.
		case r.Stale():
			stale++
			fmt.Fprintf(out, "%s: stale at %s — the origin is now at %s\n",
				r.Path, r.State.Ref, shortSHA(r.Commit))
		default:
			fmt.Fprintf(out, "%s: up to date at %s\n", r.Path, r.State.Ref)
		}
	}
	reached, failed, skipped := tally(reports)
	line := fmt.Sprintf("checked %s, %d stale", plural(reached, "vendored copy", "vendored copies"), stale)
	if stale == 0 && reached > 0 {
		line = fmt.Sprintf("checked %s, all up to date", plural(reached, "vendored copy", "vendored copies"))
	}
	printSummary(out, line, failed, skipped)
	return stale > 0 || failed > 0
}

// printUpdateReports reports an update run. A failure exits non-zero; a copy
// that was merely stale does not, because it is no longer stale.
func printUpdateReports(out, errOut io.Writer, reports []deps.Report) bool {
	written := 0
	for i := range reports {
		r := &reports[i]
		switch {
		case r.Skipped:
		case r.Err != nil:
			fmt.Fprintf(errOut, "%s: %v\n", r.Path, r.Err)
		case r.Restored:
			written++
			fmt.Fprintf(out, "%s: restored to %s — local edits discarded, make the change at its origin\n",
				r.Path, shortSHA(r.Commit))
		case r.State == nil:
			// See printCheckReports: a file the run abandoned rather than judged.
		case r.Written:
			written++
			fmt.Fprintf(out, "%s: %s → %s at %s\n",
				r.Path, shortSHA(r.State.Header.Commit), shortSHA(r.Commit), r.State.Ref)
		default:
			fmt.Fprintf(out, "%s: up to date at %s\n", r.Path, r.State.Ref)
		}
	}
	reached, failed, skipped := tally(reports)
	printSummary(out, fmt.Sprintf("updated %d of %s", written,
		plural(reached, "vendored copy", "vendored copies")), failed, skipped)

	if written == 0 {
		// Nothing arrived, so there is nothing to warn about and nothing whose
		// vocabulary can have moved under the models that import it.
		return failed > 0
	}
	for i := range reports {
		if r := &reports[i]; len(r.NewImports) > 0 {
			printTransitiveNote(out, filepath.Base(r.Path), r.NewImports, "now declares")
		}
	}
	fmt.Fprint(out, "\nRun `modelith lint` on the models that import these copies. An item a copy\n"+
		"used to define may have been renamed or removed upstream, which breaks a\n"+
		"reference this command cannot see.\n")
	printTrustWarning(errOut)
	return failed > 0
}

// tally counts what a run covered: copies reached, files that failed, and files
// that carried no provenance header.
func tally(reports []deps.Report) (reached, failed, skipped int) {
	for i := range reports {
		switch r := &reports[i]; {
		case r.Skipped:
			skipped++
		case r.Err != nil:
			failed++
		case r.State != nil:
			reached++
		}
	}
	return reached, failed, skipped
}

// printSummary closes a run with what it covered, so a user whose glob matched
// the wrong files is not told "all up to date" about nothing.
func printSummary(out io.Writer, line string, failed, skipped int) {
	if failed > 0 {
		line += fmt.Sprintf(", %s could not be reached", plural(failed, "file", "files"))
	}
	if skipped > 0 {
		line += fmt.Sprintf("; skipped %s with no provenance header", plural(skipped, "file", "files"))
	}
	fmt.Fprintf(out, "\n%s\n", line)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// shortSHA abbreviates a commit for a line a human reads. Two 40-character
// SHAs either side of an arrow is a line nobody reads, and the full value is in
// the file's header a moment later.
func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// importEntry writes a path the way an `imports:` entry is written: relative,
// slash separated, and explicitly so, so it reads as a path rather than as a
// scope.
//
// An absolute destination is relativized against wd, because an absolute import
// is a lint error — imports resolve relative to the model that declares them so
// they work in any checkout. An agent driving this command usually passes an
// absolute directory, so printing one back would hand it a line the linter
// rejects. A "../" result is fine; only an absolute one is not.
func importEntry(p, wd string) string {
	if filepath.IsAbs(p) && wd != "" {
		if rel, err := filepath.Rel(wd, p); err == nil {
			p = rel
		}
	}
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return p
	}
	return "./" + p
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
			target := out
			if target == "" {
				target = defaultOut(in)
			}

			// A target under a directory that does not exist is a misconfigured
			// -o, not a document waiting to be written. It is settled before
			// anything below can exempt a file from the check, so no exemption
			// can turn a typo into a pass.
			if check && !isDir(filepath.Dir(target)) {
				return fmt.Errorf("cannot check %s: %s is not a directory", target, filepath.Dir(target))
			}

			// A vendored model's rendered form belongs to its home repository,
			// so it arrives with no committed .md and no obligation to carry
			// one. --check runs over globs, and demanding one here would make
			// this repository regenerate somebody else's document every time
			// their model moved (ADR-0015). Naming the file to render it still
			// renders it; that is how a deep link into a vendored model's .md
			// gets something to point at — and once such an .md is committed, it
			// goes stale like any other, so from then on --check does check it.
			//
			// What stays skipped is everything the origin owns rather than this
			// repository: a copy this build cannot render at all — a newer schema
			// version, a shape this binary does not know — is not a --check
			// failure, because there is nothing here to fix. `modelith lint`
			// reports it, loudly and once.
			//
			// The exemption is claimed by a *clean* header, not by
			// provenance.Present. Present answers a deliberately broad question
			// so that a broken header still classifies a file as somebody else's
			// copy, which is right for lint: the header defects are reported and
			// the completeness noise is suppressed. Skipping on that same answer
			// would be silent, and would hand the exemption to a model this
			// repository owns that merely carries a "# modelith-" comment. An
			// exemption needs a valid claim, so a file whose header does not
			// parse cleanly is checked like any other — its defects are already
			// failing `modelith lint`.
			header, headerProblems := provenance.Parse(data)
			vendored := header != nil && len(headerProblems) == 0
			skipVendored := func(tail string) error {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is a vendored copy %s\n", in, tail)
				return nil
			}
			unrenderable := fmt.Sprintf("this modelith cannot render — skipped (run `modelith lint %s` for details)", in)
			if check && vendored {
				if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
					return skipVendored("with no committed " + filepath.Base(target) + " — skipped")
				}
			}

			// Validate against the schema first so a malformed file fails with a
			// friendly, located error rather than the raw strict-YAML parse error.
			if findings := lint.Structural(data); len(findings) > 0 {
				if check && vendored {
					return skipVendored(unrenderable)
				}
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
				if check && vendored {
					return skipVendored(unrenderable)
				}
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

// isDir reports whether p exists and is a directory. Anything else — missing,
// a plain file, unreadable — is false, because every one of those means a
// target under p is not merely uncommitted.
func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
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
