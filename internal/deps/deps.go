// Package deps acquires a model from another repository and stamps it as a
// vendored copy.
//
// It implements no network transport of its own. Every fetch is delegated to
// the gh CLI, executed as an argv array and never through a shell, so this
// binary holds no TLS configuration and no credentials (ADR-0011).
package deps

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/provenance"
)

// Runner runs an external command and returns its standard output. It is the
// seam the gh calls go through, so Import is testable without a network.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run executes name with args and returns its standard output. Standard error
// is folded into the returned error, because gh reports why it refused there.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s is not installed — modelith delegates fetching to it; install it from https://cli.github.com and run `%s auth login`", name, name)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// Source is a model file in another repository, as an origin URL parsed into
// the parts a fetch and a later refresh both need.
type Source struct {
	Origin string // https://github.com/owner/repo
	Owner  string
	Repo   string
	Ref    string
	Path   string // path within the repository
}

// ParseSource reads a GitHub blob URL — the address of the file as it appears
// in a browser — into its parts. A non-empty ref overrides the one in the URL,
// which is also how a branch whose name contains a slash is disambiguated from
// the path that follows it.
func ParseSource(raw, ref string) (Source, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Source{}, fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	if u.Host != "github.com" {
		return Source{}, fmt.Errorf(
			"modelith can currently fetch only from github.com, and %q is on %q. Support for other hosts is not written yet because nobody has needed it — if you do, please open an issue at %s saying where your models live",
			raw, u.Host, issuesURL)
	}
	// A URL copied from the browser often carries ?plain=1 and a #L12 anchor.
	// Neither is part of the file's address.
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "blob" {
		return Source{}, fmt.Errorf(
			"%q is not a GitHub file URL — it should look like https://github.com/<owner>/<repo>/blob/<ref>/<path to the .modelith.yaml>, which is the address you get by opening the file on github.com",
			raw)
	}
	src := Source{
		Owner:  parts[0],
		Repo:   parts[1],
		Origin: "https://github.com/" + parts[0] + "/" + parts[1],
	}
	rest := strings.Join(parts[3:], "/")
	// A ref may contain slashes ("release/v2"), and the URL gives no way to tell
	// where it ends. Splitting at the first segment is right for the common
	// case; an explicit ref settles the rest.
	switch {
	case ref != "" && strings.HasPrefix(rest, ref+"/"):
		src.Ref, src.Path = ref, strings.TrimPrefix(rest, ref+"/")
	default:
		src.Ref, src.Path, _ = strings.Cut(rest, "/")
		if ref != "" {
			src.Ref = ref
		}
	}
	if src.Path == "" {
		return Source{}, fmt.Errorf("%q names no file inside the repository", raw)
	}
	return src, nil
}

const issuesURL = "https://github.com/stacklok/modelith/issues"

// Options are the inputs to Import.
type Options struct {
	// URL is the GitHub blob URL of the model to vendor.
	URL string
	// Dir is the directory to write into. Empty means the working directory.
	Dir string
	// Ref overrides the ref in URL.
	Ref string
	// Now stamps the header's imported date, in local time.
	Now time.Time
	// Run is the command seam; nil uses ExecRunner.
	Run Runner
}

// Result reports what Import did, so the caller can print it.
type Result struct {
	// Path is where the vendored copy was written.
	Path string
	// Header is what was stamped into it.
	Header *provenance.Header
	// Replaced says an earlier copy was overwritten.
	Replaced bool
	// TheirImports are the fetched model's own imports. Vendoring fetches one
	// file, so these were not followed; the caller says so.
	TheirImports []string
}

// Import fetches the model at opts.URL and writes it into opts.Dir as a
// vendored copy carrying a provenance header.
//
// It writes the file and nothing else. The importing model's `imports:` list is
// the caller's to edit: rewriting a user's YAML costs comment and formatting
// fidelity, and a repository with several models gives no way to guess which
// one meant to import this. That second, manual step is also the gate on the
// injection risk vendoring carries (ADR-0014) — the file is inert until it is
// named in an imports list.
func Import(ctx context.Context, opts Options) (*Result, error) {
	src, err := ParseSource(opts.URL, opts.Ref)
	if err != nil {
		return nil, err
	}
	runner := opts.Run
	if runner == nil {
		runner = ExecRunner{}
	}

	content, err := fetchContent(ctx, runner, src)
	if err != nil {
		return nil, err
	}
	if provenance.Present(content) {
		return nil, fmt.Errorf(
			"%s is itself a vendored copy — it carries a provenance header naming where it came from. Vendor it from that origin instead, so this repository tracks the model's home rather than somebody else's copy of it",
			opts.URL)
	}
	m, err := model.Parse(content)
	if err != nil || m.Kind != "DomainModel" {
		return nil, fmt.Errorf("%s is not a domain model — check that the URL names a *.modelith.yaml file", opts.URL)
	}

	commit, err := fetchCommit(ctx, runner, src)
	if err != nil {
		return nil, err
	}

	h := &provenance.Header{
		Vendored: provenance.Banner,
		Fetch:    "git",
		Origin:   src.Origin,
		Path:     src.Path,
		Ref:      src.Ref,
		Commit:   commit,
		Imported: opts.Now.Format("2006-01-02"),
		Digest:   provenance.Digest(content),
	}

	target := filepath.Join(opts.Dir, path.Base(src.Path))
	_, statErr := os.Stat(target)
	replaced := statErr == nil
	if err := os.WriteFile(target, provenance.Stamp(content, h), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", target, err)
	}

	res := &Result{Path: target, Header: h, Replaced: replaced}
	for _, imp := range m.Imports {
		res.TheirImports = append(res.TheirImports, imp.Path)
	}
	return res, nil
}

// fetchContent returns the file's bytes. The raw media type asks the API for
// the content itself rather than a JSON envelope carrying it base64-encoded, so
// nothing here has to decode.
func fetchContent(ctx context.Context, runner Runner, src Source) ([]byte, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/contents/%s?ref=%s",
		src.Owner, src.Repo, escapePath(src.Path), url.QueryEscape(src.Ref))
	out, err := runner.Run(ctx, "gh", "api", "-H", "Accept: application/vnd.github.raw", endpoint)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// fetchCommit returns the commit that last touched the file at this ref.
//
// That, rather than the head of the ref, is what identifies the version of this
// file: it does not move when unrelated commits land on the branch, so a later
// freshness check reports the model changing rather than the repository being
// busy.
func fetchCommit(ctx context.Context, runner Runner, src Source) (string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/commits?path=%s&sha=%s&per_page=1",
		src.Owner, src.Repo, url.QueryEscape(src.Path), url.QueryEscape(src.Ref))
	out, err := runner.Run(ctx, "gh", "api", endpoint, "--jq", ".[0].sha")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" || sha == "null" {
		return "", fmt.Errorf("%s/%s has no commit touching %q at %q", src.Owner, src.Repo, src.Path, src.Ref)
	}
	return sha, nil
}

// escapePath escapes each segment of a repository path, leaving the separators
// alone so the API still sees a path.
func escapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}
