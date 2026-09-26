// Package deps acquires a model from another repository and stamps it as a
// vendored copy.
//
// Every fetch is delegated to an external CLI — gh for GitHub, az for Azure
// DevOps — executed as an argv array and never through a shell, so this
// binary holds no TLS configuration and no credentials (ADR-0011).
package deps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// Host identifies the source-code platform a Source was parsed from.
type Host string

const (
	HostGitHub Host = "github"
	HostADO    Host = "azure-devops"
)

// Runner runs an external command and returns its standard output. It is the
// seam the gh calls go through, so Import is testable without a network.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// runCommand executes cmd and folds its outcome into an error, because the
// CLI reports why it refused on standard error. It is the shared tail of the
// platform-specific Run methods in exec_unix.go and exec_windows.go, so the
// error text cannot drift between platforms.
func runCommand(cmd *exec.Cmd, name string, args []string, stderr *strings.Builder) ([]byte, error) {
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, unusable{fmt.Errorf("%s is not installed — modelith delegates fetching to it%s", name, installHint(name))}
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			failed := fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
			if unauthenticated(msg) {
				return nil, unusable{failed}
			}
			return nil, failed
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// installHint returns the rest of the "not installed" message for a given CLI.
func installHint(name string) string {
	switch name {
	case "gh":
		return "; install it from https://cli.github.com and run `gh auth login`"
	case "az":
		return "; install it from https://aka.ms/azure-cli and run `az login`"
	}
	return ""
}

// timeoutRunner bounds each delegated command to timeout. It is a decorator on
// the Runner seam: the deadline is enforced per command, so a slow-but-working
// content fetch does not consume the commit fetch's budget.
type timeoutRunner struct {
	inner   Runner
	timeout time.Duration
}

// Run derives a per-command deadline from the caller's context and abandons the
// command when it expires. The error names the command and the bound and tells
// the user how to adjust it; it deliberately does not echo the argv, so a URI
// cannot leak into logs through this path.
func (r timeoutRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out, err := r.inner.Run(ctx, name, args...)
	// ErrWaitDelay is the deadline's last line of defense: the direct child was
	// killed, but a helper it spawned held the pipe past WaitDelay, so Wait
	// abandoned it. It is the deadline surfacing, so report it as one.
	//
	// The deadline's main line is ctx.Err(), asked for a *deadline* specifically:
	// when the bound fires, ExecRunner SIGKILLs the process group and cmd.Output
	// returns *exec.ExitError ("signal: killed") — os/exec prefers the process's
	// own error over the context's, so errors.Is cannot see the deadline through
	// it (issue #3). The context still distinguishes a fired deadline from a
	// caller canceling the fetch (e.g. Ctrl+C); only the former is a timeout the
	// user can raise with --timeout. And err must be non-nil: a command that
	// completed successfully just before the bound must not have its output
	// discarded because the deadline ticked over in the same instant.
	if err != nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, exec.ErrWaitDelay)) {
		return nil, fmt.Errorf("%s did not finish within %s — the fetch was abandoned (raise it with --timeout if this is a slow but legitimate fetch)", name, r.timeout)
	}
	return out, err
}

// unauthenticated reports whether gh refused for want of credentials rather
// than because of anything about the request.
//
// It matches gh's own text because gh is a separate program: it reports this on
// stderr and exits 1, with no typed error to unwrap and no distinguishing exit
// code. The three needles are the ones gh's binary actually carries — "gh auth
// login" appears in every logged-out message, "GH_TOKEN" in the two automation
// ones, and HTTP 401 is a token that exists and is refused. Reading it wrong
// costs a batch that stops when it could have continued, or one that repeats
// the same paragraph per file.
func unauthenticated(msg string) bool {
	for _, needle := range []string{"gh auth login", "GH_TOKEN", "HTTP 401"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// unusable marks a failure of the tool itself rather than of the request, so
// errors.Is(err, ErrToolUnavailable) reports true without the sentinel's text
// appearing in the message the user reads.
type unusable struct{ error }

func (unusable) Is(target error) bool { return target == ErrToolUnavailable }

// Source is a model file in another repository, as an origin URL parsed into
// the parts a fetch and a later refresh both need.
type Source struct {
	Host    Host   // HostGitHub or HostADO
	Origin  string // https://github.com/owner/repo or https://dev.azure.com/org/project/_git/repo
	Owner   string // GitHub owner, or ADO organization
	Project string // ADO project; empty for GitHub
	Repo    string
	Ref     string
	RefType string // ADO version type: "branch", "tag", or "commit"; empty for GitHub
	Path    string // path within the repository
}

// ParseSource reads a GitHub or Azure DevOps blob URL — the address of the file
// as it appears in a browser — into its parts. A non-empty ref overrides the
// one in the URL.
func ParseSource(raw, ref string) (Source, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Source{}, fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	host := normalizeHost(u.Host)
	// The Raw button hands back this host, so it is an easy thing to paste. The
	// model is on GitHub and the address just names a different view of it, so
	// sending the reader off to ask for another host to be supported would be
	// advice about the wrong problem.
	if host == "raw.githubusercontent.com" {
		return Source{}, fmt.Errorf(
			"%q is a raw file URL, and modelith wants the page you see when you open the file on github.com — the same address with /blob/ in it. Open the file there and copy the address bar",
			raw)
	}

	switch {
	case host == "github.com":
		return parseGitHubSource(u, ref)
	case host == "dev.azure.com":
		return parseADOSource(u, ref)
	case strings.HasSuffix(host, ".visualstudio.com"):
		org := strings.TrimSuffix(host, ".visualstudio.com")
		return Source{}, fmt.Errorf(
			"%q is a legacy visualstudio.com URL — Azure DevOps moved to dev.azure.com. Open the file in your browser on dev.azure.com (e.g. https://dev.azure.com/%s) and import that address instead",
			raw, org)
	default:
		return Source{}, fmt.Errorf(
			"modelith can currently fetch only from github.com and dev.azure.com, and %q is on %q. Support for other hosts is not written yet because nobody has needed it — if you do, please open an issue at %s saying where your models live",
			raw, u.Host, issuesURL)
	}
}

// parseGitHubSource parses a GitHub blob URL:
//
//	https://github.com/<owner>/<repo>/blob/<ref>/<path>
func parseGitHubSource(u *url.URL, ref string) (Source, error) {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "blob" {
		return Source{}, fmt.Errorf(
			"%q is not a GitHub file URL — it should look like https://github.com/<owner>/<repo>/blob/<ref>/<path to the .modelith.yaml>, which is the address you get by opening the file on github.com",
			u.String())
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return Source{}, fmt.Errorf(
				"%q has a %q path segment, which is not part of a file's address on github.com — copy the address bar from the file's page rather than assembling the URL by hand",
				u.String(), p)
		}
	}
	src := Source{
		Host:   HostGitHub,
		Owner:  parts[0],
		Repo:   parts[1],
		Origin: "https://github.com/" + parts[0] + "/" + parts[1],
	}
	rest := strings.Join(parts[3:], "/")
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
		return Source{}, fmt.Errorf("%q names no file inside the repository", u.String())
	}
	return src, nil
}

// parseADOSource parses an Azure DevOps blob URL:
//
//	https://dev.azure.com/<org>/<project>/_git/<repo>?path=<path>&version=GB<branch>
//
// The version parameter prefix indicates: GB=GitBranch, GT=GitTag, GC=GitCommit.
func parseADOSource(u *url.URL, ref string) (Source, error) {
	// Path: /<org>/<project>/_git/<repo>
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "_git" {
		return Source{}, fmt.Errorf(
			"%q is not an Azure DevOps file URL — it should look like https://dev.azure.com/<org>/<project>/_git/<repo>?path=<path>&version=GB<branch>, which is the address you get by opening the file on dev.azure.com",
			u.String())
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return Source{}, fmt.Errorf(
				"%q has a %q path segment, which is not part of a file's address on dev.azure.com — copy the address bar from the file's page rather than assembling the URL by hand",
				u.String(), p)
		}
	}

	q := u.Query()
	filePath := strings.TrimPrefix(strings.TrimSpace(q.Get("path")), "/")
	if filePath == "" {
		return Source{}, fmt.Errorf("%q names no file inside the repository — it needs a ?path= query parameter", u.String())
	}
	for _, p := range strings.Split(filePath, "/") {
		if p == "" || p == "." || p == ".." || strings.Contains(p, "\\") || strings.TrimSpace(p) != p {
			return Source{}, fmt.Errorf(
				"%q has a %q path segment, which is not part of a file's address on dev.azure.com — copy the address bar from the file's page rather than assembling the URL by hand",
				u.String(), p)
		}
	}

	version := q.Get("version")
	var urlRef, refType string
	switch {
	case strings.HasPrefix(version, "GB"):
		urlRef, refType = version[2:], "branch"
	case strings.HasPrefix(version, "GT"):
		urlRef, refType = version[2:], "tag"
	case strings.HasPrefix(version, "GC"):
		urlRef, refType = version[2:], "commit"
	case version != "":
		urlRef = version
	}

	src := Source{
		Host:    HostADO,
		Owner:   parts[0],
		Project: parts[1],
		Repo:    parts[3],
		Origin:  "https://dev.azure.com/" + parts[0] + "/" + parts[1] + "/_git/" + parts[3],
		Path:    filePath,
		Ref:     urlRef,
		RefType: refType,
	}
	if ref != "" {
		src.Ref = ref
		src.RefType = "" // let the ADO API auto-detect the ref type
	}
	if src.Ref == "" {
		return Source{}, fmt.Errorf(
			"%q has no version parameter — add &version=GB<branch> to the URL, or pass --ref to pin a specific ref",
			u.String())
	}
	return src, nil
}

const issuesURL = "https://github.com/stacklok/modelith/issues"

// Options are the inputs to Import.
type Options struct {
	// URL is the GitHub or Azure DevOps blob URL of the model to vendor.
	URL string
	// Dir is the directory to write into. Empty means the working directory.
	Dir string
	// Ref overrides the ref in URL.
	Ref string
	// Now stamps the header's imported date, in local time.
	Now time.Time
	// Timeout bounds each delegated command (gh, az) individually. Zero means
	// no bound: a hung CLI becomes a fast, actionable error instead of a
	// silent wait.
	Timeout time.Duration
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
	if opts.Timeout > 0 {
		runner = timeoutRunner{inner: runner, timeout: opts.Timeout}
	}

	var content []byte
	var commit string

	if src.Host == HostADO {
		// ADO fetch delegates to the az CLI.
		content, err = fetchContentADO(ctx, runner, src)
		if err != nil {
			return nil, fmt.Errorf("fetching content: %w%s", err, splitHint(src, err))
		}
		commit, err = fetchCommitADO(ctx, runner, src)
		if err != nil {
			return nil, fmt.Errorf("fetching commit: %w", err)
		}
	} else {
		// GitHub fetch delegates to the gh CLI.
		content, err = fetchContent(ctx, runner, src)
		if err != nil {
			return nil, fmt.Errorf("fetching content: %w%s", err, splitHint(src, err))
		}
		commit, err = fetchCommit(ctx, runner, src)
		if err != nil {
			return nil, fmt.Errorf("fetching commit: %w", err)
		}
	}

	if provenance.Present(content) {
		return nil, fmt.Errorf(
			"%s carries a %s line, so modelith reads it as somebody else's copy rather than a model's home. If it is a copy, vendor it from the origin its header names instead, so this repository tracks the model's home. If it is not — the line is an ordinary comment that happens to use modelith's reserved prefix at column zero — it has to be indented or removed at the origin before this file can be vendored",
			opts.URL, provenance.LinePrefix)
	}
	m, err := model.Parse(content)
	if err != nil {
		return nil, fmt.Errorf(
			"%s did not parse as a domain model — check that the URL names a *.modelith.yaml file, and that this modelith is new enough to read it: %w",
			opts.URL, err)
	}
	if m.Kind != "DomainModel" {
		declares := fmt.Sprintf("declares kind %q", m.Kind)
		if m.Kind == "" {
			declares = "declares no kind"
		}
		return nil, fmt.Errorf("%s is not a domain model — it %s, not \"DomainModel\"", opts.URL, declares)
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
	replaced, err := guardTarget(target, src)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, provenance.Stamp(content, h), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", target, err)
	}

	res := &Result{Path: target, Header: h, Replaced: replaced}
	for _, imp := range m.Imports {
		res.TheirImports = append(res.TheirImports, imp.Path)
	}
	return res, nil
}

// guardTarget decides whether writing to target is safe, and reports whether an
// existing copy is being replaced.
//
// The destination filename comes from the origin, so it can collide with a file
// this repository already has: a model of its own, or a copy of a different
// model that happens to share a basename. Import refuses to fetch a file that is
// already somebody else's copy; the same care is owed to what it writes over,
// because a model this repository wrote is not recoverable by fetching again.
func guardTarget(target string, src Source) (replaced bool, err error) {
	existing, readErr := os.ReadFile(target)
	if errors.Is(readErr, fs.ErrNotExist) {
		return false, nil
	}
	if readErr != nil {
		return false, fmt.Errorf("reading %s: %w", target, readErr)
	}
	if !provenance.Present(existing) {
		return false, fmt.Errorf(
			"%s already exists and carries no provenance header, so it is a model this repository owns rather than a copy of one — importing would overwrite it. Import into a different directory, or move that file aside first",
			target)
	}
	// A malformed header can still be repaired, but only if its surviving
	// identity names this exact source. A reserved-prefix comment alone is not
	// enough to establish that this repository does not own the file.
	h, _ := provenance.Parse(existing)
	if h.Origin == "" || h.Path == "" {
		return false, fmt.Errorf(
			"%s cannot be identified as a copy of the source (%s/%s) because its provenance header does not name both an origin and path. Import into a different directory, or deliberately move or delete the existing file first",
			target, src.Origin, src.Path)
	}
	switch {
	// GitHub treats an owner and a repository name case-insensitively, and
	// Origin keeps whatever casing the URL was typed with, so comparing these
	// byte-for-byte would refuse a refresh over nothing but capitalisation.
	case !strings.EqualFold(normOrigin(h.Origin), normOrigin(src.Origin)):
		return false, fmt.Errorf(
			"%s is a vendored copy of %s/%s, not of %s/%s — two different models share that filename. Import into a different directory so both can live here",
			target, h.Origin, h.Path, src.Origin, src.Path)
	case h.Path != src.Path:
		// Same repository, different path: either the model moved upstream or
		// this repository has two models from it sharing a basename. Only the
		// user knows which, and the two remedies are different.
		return false, fmt.Errorf(
			"%s is a vendored copy of %s in %s, and you are importing %s from the same repository. If the model moved, delete %s and import again; if these are two different models, import into a different directory so both can live here",
			target, h.Path, h.Origin, src.Path, target)
	}
	return true, nil
}

// splitHint explains the one way ParseSource can be wrong about a URL it
// accepted. For GitHub sources, a browse URL gives no way to tell where a ref
// containing a slash ends and the path begins, so the split is taken at the
// first segment; a failed fetch is where that guess surfaces, as a 404. ADO
// URLs carry the ref in a query parameter, so this ambiguity does not arise.
//
// It is offered only for that failure. A missing tool, a rejected credential,
// an unreachable network — none of those say anything about the URL.
func splitHint(src Source, err error) string {
	// The ref/path ambiguity is GitHub-specific; ADO URLs use query params.
	if src.Host == HostADO {
		return ""
	}
	if !strings.Contains(src.Path, "/") || !isNotFound(err) {
		return ""
	}
	return fmt.Sprintf(
		"\n\nmodelith read %q as the ref and %q as the path. If the ref has a slash in it that split is wrong: pass the whole ref to --ref, or — if --ref is already pinning a different ref, which cannot re-split the path — open the file on the ref you want and import that URL instead",
		src.Ref, src.Path)
}

// isNotFound reports whether err says the endpoint does not exist.
func isNotFound(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(msg, "Not Found")
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

// normOrigin puts an origin in the form ParseSource writes, so that a header
// someone typed by hand is read as naming the repository it names. A trailing
// slash is the difference a browser's address bar most readily introduces, and
// every place an origin is compared or rebuilt has to agree on it: disagreeing
// makes the same header refresh cleanly under one command and be refused as a
// different model's by another.
func normOrigin(o string) string { return strings.TrimSuffix(o, "/") }

// normalizeHost puts a URL host in the form a comparison uses: a host is
// case-insensitive, and a browser hands back the "www." form as readily as the
// bare one, so neither names a different site. ParseSource dispatches on this
// form, and any other caller reasoning about an origin's host must agree with
// it — comparing a raw host against "github.com" would refuse "www.github.com"
// and accept nothing a dispatch would.
func normalizeHost(host string) string {
	return strings.TrimPrefix(strings.ToLower(host), "www.")
}

// originHost reports the host an origin URL names, in the normalized form
// normalizeHost produces, or "" when the origin does not parse. It is for
// callers that hold only the recorded origin — refresh, deciding whether it can
// reach the address it is about to rebuild — and must not guess the host by
// string inspection.
func originHost(origin string) string {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return ""
	}
	return normalizeHost(u.Host)
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

// --- Azure DevOps transport (az rest) ---

// adoResourceID is the Azure DevOps application ID that az rest needs to
// request an AAD token with the correct audience when the URL alone doesn't
// let it derive one.
const adoResourceID = "499b84ac-1321-427f-aa17-267ca6975798"

// adoVersionType returns the versionDescriptor.versionType for a Source. When
// the version prefix was not one of the known three (GB/GT/GC), the API
// endpoint accepts an empty versionType and auto-detects the ref.
func adoVersionType(src Source) string {
	switch src.RefType {
	case "branch", "tag", "commit":
		return src.RefType
	}
	return ""
}

// fetchContentADO fetches the file content from Azure DevOps by delegating to
// `az rest`. Auth is handled by the az CLI.
//
// The body is written to a temporary file with --output-file and read back
// rather than taken from stdout: az rest appends a newline when it prints a
// raw body to stdout, so the stdout form is not byte-identical to the origin
// file — it drifts a trailing newline into the vendored copy and its digest
// (the ADR-0015 amendment on the Azure DevOps transport). The --output-file form
// is the exact API response body.
func fetchContentADO(ctx context.Context, runner Runner, src Source) ([]byte, error) {
	uri := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/git/repositories/%s/items?path=%s&versionDescriptor.version=%s&api-version=7.1",
		url.PathEscape(src.Owner), url.PathEscape(src.Project),
		url.PathEscape(src.Repo), url.QueryEscape(src.Path),
		url.QueryEscape(src.Ref))
	if vt := adoVersionType(src); vt != "" {
		uri += "&versionDescriptor.versionType=" + url.QueryEscape(vt)
	}

	tmp, err := os.CreateTemp("", "modelith-ado-*")
	if err != nil {
		return nil, fmt.Errorf("creating a temp file for the fetch: %w", err)
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return nil, fmt.Errorf("closing the temp file for the fetch: %w", err)
	}
	defer func() { _ = os.Remove(name) }()

	if _, err := runner.Run(ctx, "az", "rest", "--method", "get",
		"--resource", adoResourceID, "--uri", uri,
		"--output-file", name); err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}

// fetchCommitADO returns the commit that last touched the file at the given
// ref by delegating to `az rest`. Auth is handled by the az CLI.
func fetchCommitADO(ctx context.Context, runner Runner, src Source) (string, error) {
	uri := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/git/repositories/%s/commits?searchCriteria.itemPath=%s&searchCriteria.itemVersion.version=%s&$top=1&api-version=7.1",
		url.PathEscape(src.Owner), url.PathEscape(src.Project),
		url.PathEscape(src.Repo), url.QueryEscape(src.Path),
		url.QueryEscape(src.Ref))
	if vt := adoVersionType(src); vt != "" {
		uri += "&searchCriteria.itemVersion.versionType=" + url.QueryEscape(vt)
	}

	out, err := runner.Run(ctx, "az", "rest", "--method", "get",
		"--resource", adoResourceID, "--uri", uri,
		"--query", "value[0].commitId", "-o", "tsv")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" || sha == "null" {
		return "", fmt.Errorf("%s/%s/_git/%s has no commit touching %q at %q",
			src.Owner, src.Project, src.Repo, src.Path, src.Ref)
	}
	return sha, nil
}
