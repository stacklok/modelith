package deps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/stacklok/modelith/internal/model"
	"github.com/stacklok/modelith/internal/provenance"
)

// ErrToolUnavailable marks a failure of gh itself rather than of the request:
// it is not installed, or it holds no usable credentials. Every file in a run
// would fail it identically, so a batch stops on it instead of repeating the
// same paragraph once per copy.
var ErrToolUnavailable = errors.New("gh is unavailable")

// State is one vendored copy measured against its origin.
//
// The questions below are deliberately distinct and no two of them share a
// comparison. Collapsing questions about a vendored file has caused a bug once
// already (ADR-0015's provenance rule). Every comparison here runs through
// provenance.Digest, which strips header lines, so all of them are content
// against content and none is perturbed by the header a refresh rewrites.
type State struct {
	// Path is the copy on disk, as it was named on the command line.
	Path string
	// Header is what that copy records about where it came from.
	Header *provenance.Header
	// Ref is the ref consulted: the header's, or the override.
	Ref string
	// Local is the bytes on disk. Upstream is what the origin serves now.
	Local, Upstream []byte
	// Model is the origin's current content, parsed.
	Model *model.Model
}

// Moved reports whether the origin has changed since this copy was taken, which
// is the question deps check answers. It compares against the recorded digest,
// which import wrote over the upstream bytes before stamping — so this holds
// whether or not the copy on disk was edited (ADR-0016).
func (s *State) Moved() bool { return provenance.Digest(s.Upstream) != s.Header.Digest }

// Edited reports whether the copy on disk drifted from the version its own
// header claims. lint answers this offline and it is not check's business; it
// is here because it is half of why an update writes.
func (s *State) Edited() bool { return provenance.Digest(s.Local) != s.Header.Digest }

// Repinned reports whether the ref being consulted is not the recorded one.
func (s *State) Repinned() bool { return s.Ref != s.Header.Ref }

// Current reports whether the copy on disk already holds what the origin
// serves. With Repinned it is the whole write condition for deps update: a
// hand-edited copy is not current whatever the origin did, so updating it
// restores it, and no case needs a branch of its own (ADR-0016).
func (s *State) Current() bool { return provenance.Digest(s.Local) == provenance.Digest(s.Upstream) }

// Report is what happened to one file in a check or update run.
type Report struct {
	// Path is the file as it was named on the command line.
	Path string
	// Skipped says the file carries no provenance header, so it is a model this
	// repository owns and neither command has anything to say about it.
	Skipped bool
	// Err is why this file could not be checked. The run continues past it.
	Err error
	// State is nil when the file was skipped or failed.
	State *State
	// Written says update rewrote the file. Restored distinguishes the two
	// reasons it might have: a new version arrived, or only the copy on disk
	// had drifted from the version it claims.
	Written, Restored bool
	// Commit is the origin's commit for this file. It is resolved only when
	// something moved, so a clean check costs one gh call per copy, not two.
	Commit string
	// NewImports are imports a refresh brought that the copy on disk did not
	// have. Vendoring is not transitive, so they are reported, never followed.
	NewImports []string
}

// Stale reports whether this file needs updating, which is the verdict deps
// check prints and the condition its exit code carries.
func (r *Report) Stale() bool { return r.State != nil && r.State.Moved() }

// CheckOptions are the inputs to Check.
type CheckOptions struct {
	// Paths are the files to check. One that carries no provenance header is
	// skipped, because the glob a user passes to lint holds their own models too.
	Paths []string
	// Run is the command seam; nil uses ExecRunner.
	Run Runner
}

// UpdateOptions are the inputs to Update.
type UpdateOptions struct {
	// Paths are the files to update.
	Paths []string
	// Ref re-pins the copy, overriding the ref its header records. It applies to
	// exactly one file: one ref across several unrelated origins is meaningless.
	Ref string
	// Now stamps a refreshed header's imported date, in local time.
	Now time.Time
	// Run is the command seam; nil uses ExecRunner.
	Run Runner
}

// Check reports, for each path, whether the vendored copy there has fallen
// behind its origin. It writes nothing, which is what makes it safe to run in
// CI and against a read-only checkout.
//
// A per-file failure lands in that file's Report and the run continues. A
// non-nil error means the run stopped early — only gh being unusable does that.
func Check(ctx context.Context, opts CheckOptions) ([]Report, error) {
	return survey(ctx, surveyOptions{paths: opts.Paths, run: opts.Run})
}

// Update brings each vendored copy forward to what its origin serves now.
//
// It writes when the copy on disk does not already hold the origin's content,
// or when the ref moved. It does not touch the model that imports the copy: an
// item this copy used to define may have been renamed or removed upstream, and
// modelith lint is what reports that, from the importing model's seat.
func Update(ctx context.Context, opts UpdateOptions) ([]Report, error) {
	if opts.Ref != "" && len(opts.Paths) != 1 {
		return nil, fmt.Errorf(
			"--ref re-pins one copy and %d files were named — a single ref across several origins names a different version in each. Run it once per copy",
			len(opts.Paths))
	}
	return survey(ctx, surveyOptions{paths: opts.Paths, ref: opts.Ref, now: opts.Now, run: opts.Run, write: true})
}

type surveyOptions struct {
	paths []string
	ref   string
	now   time.Time
	run   Runner
	write bool
}

func survey(ctx context.Context, opts surveyOptions) ([]Report, error) {
	runner := opts.run
	if runner == nil {
		runner = ExecRunner{}
	}
	reports := make([]Report, 0, len(opts.paths))
	for _, p := range opts.paths {
		rep, err := visit(ctx, runner, p, opts)
		if err != nil {
			// gh itself is unusable, so every file left would fail identically.
			// Stop, and hand back what was learned before this one. The file
			// that hit it gets no Report: it was abandoned mid-flight, so it has
			// neither a verdict nor a per-file failure to report, and the error
			// returned here is the whole story.
			return reports, err
		}
		reports = append(reports, rep)
	}
	return reports, nil
}

// visit measures one file and, when asked to write, writes it. It returns a
// non-nil error only to abort the whole run; everything else is a Report.
func visit(ctx context.Context, runner Runner, path string, opts surveyOptions) (Report, error) {
	rep := Report{Path: path}

	local, err := os.ReadFile(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			// The Report already carries the path and the caller prints it, so
			// the PathError's own copy of it would be the second in one line.
			err = fmt.Errorf("cannot be read: %w", pe.Err)
		}
		rep.Err = err
		return rep, nil
	}
	if !provenance.Present(local) {
		rep.Skipped = true
		return rep, nil
	}

	// Classification is generous and exemption is strict (ADR-0015): the file
	// above is a copy however broken its header, but a header with any defect in
	// it earns no verdict here. Every comparison below rests on the recorded
	// digest, and a header whose digest is malformed or whose origin is missing
	// would produce a confident answer built on nothing.
	h, problems := provenance.Parse(local)
	if len(problems) > 0 {
		rep.Err = fmt.Errorf(
			"its provenance header is not usable (%s) — run `modelith lint %s` for the full report",
			problems[0].Message, path)
		return rep, nil
	}

	ref := h.Ref
	if opts.ref != "" {
		ref = opts.ref
	}
	src, err := sourceFromHeader(h, ref)
	if err != nil {
		rep.Err = err
		return rep, nil
	}

	upstream, err := fetchContent(ctx, runner, src)
	if err != nil {
		if errors.Is(err, ErrToolUnavailable) {
			return rep, err
		}
		rep.Err = fmt.Errorf("%w%s", err, movedHint(src, err))
		return rep, nil
	}
	m, err := checkFetched(upstream, src)
	if err != nil {
		rep.Err = err
		return rep, nil
	}

	st := &State{Path: path, Header: h, Ref: ref, Local: local, Upstream: upstream, Model: m}
	rep.State = st

	if !opts.write {
		// The commit is reporting, not verdict: resolve it only when the origin
		// actually moved, so a clean check costs one call per copy (ADR-0016).
		if st.Moved() {
			commit, err := fetchCommit(ctx, runner, src)
			if err != nil {
				if errors.Is(err, ErrToolUnavailable) {
					return rep, err
				}
				rep.Err = err
			}
			rep.Commit = commit
		}
		return rep, nil
	}

	switch {
	case st.Current() && !st.Repinned():
		return rep, nil

	case !st.Moved() && !st.Repinned():
		// A restore. The origin still holds the version this header describes
		// and only the copy on disk drifted, so re-stamping the recorded header
		// reproduces the bytes the original import wrote: no commit is fetched,
		// because nothing moved, and imported: does not change, because no new
		// version arrived (ADR-0016).
		if err := write(path, upstream, h); err != nil {
			rep.Err = err
			return rep, nil
		}
		rep.Written, rep.Restored, rep.Commit = true, true, h.Commit
		return rep, nil
	}

	// A refresh: a new version, a new pin, or both.
	commit, err := fetchCommit(ctx, runner, src)
	if err != nil {
		if errors.Is(err, ErrToolUnavailable) {
			return rep, err
		}
		rep.Err = err
		return rep, nil
	}
	next := *h
	next.Ref = ref
	next.Commit = commit
	next.Imported = opts.now.Format("2006-01-02")
	next.Digest = provenance.Digest(upstream)
	if err := write(path, upstream, &next); err != nil {
		rep.Err = err
		return rep, nil
	}
	rep.Written, rep.Commit = true, commit
	rep.NewImports = newImports(local, m)
	return rep, nil
}

func write(path string, content []byte, h *provenance.Header) error {
	if err := os.WriteFile(path, provenance.Stamp(content, h), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// sourceFromHeader rebuilds the fetch address from what a header records.
//
// It reassembles the blob URL and hands it back to ParseSource rather than
// picking the origin apart here, so a hand-written header gets the same
// treatment a typed one does: the host check, and the dot-segment rejection
// that escapePath depends on to keep an endpoint inside the repository's
// contents namespace. Passing ref explicitly is what lets a ref containing a
// slash split correctly, which is knowable here and is not from a URL alone.
func sourceFromHeader(h *provenance.Header, ref string) (Source, error) {
	if h.Fetch != "git" {
		// Unreachable while git is the only method, but a header naming a
		// method a future build understands would parse without problems and
		// still have no address this function can assemble.
		return Source{}, fmt.Errorf(
			"modelith cannot refresh a copy fetched with %q — this build knows how to fetch %s",
			h.Fetch, strings.Join(provenance.Methods(), ", "))
	}
	return ParseSource(fmt.Sprintf("%s/blob/%s/%s", strings.TrimSuffix(h.Origin, "/"), ref, h.Path), ref)
}

// checkFetched applies to a refetch the same two refusals import applies to a
// first fetch, because the file at an address is not the file that was there
// last time.
func checkFetched(upstream []byte, src Source) (*model.Model, error) {
	if provenance.Present(upstream) {
		return nil, fmt.Errorf(
			"%s now carries a %s line at its origin, so modelith reads it as somebody else's copy rather than a model's home. Vendoring a copy of a copy would record the wrong repository as this model's home — vendor it from the origin its own header names instead",
			src.Path, provenance.LinePrefix)
	}
	m, err := model.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf(
			"%s no longer parses as a domain model at its origin, so the copy here was left alone: %w",
			src.Path, err)
	}
	if m.Kind != "DomainModel" {
		declares := fmt.Sprintf("declares kind %q", m.Kind)
		if m.Kind == "" {
			declares = "declares no kind"
		}
		return nil, fmt.Errorf(
			"%s is no longer a domain model at its origin — it %s, not \"DomainModel\" — so the copy here was left alone",
			src.Path, declares)
	}
	return m, nil
}

// movedHint explains the failure a refetch has that a first fetch does not: the
// address came from a header written some time ago, and a model can move or be
// deleted in its own repository without anything here noticing.
//
// It is offered only for a 404. A rejected credential or an unreachable network
// says nothing about where the file lives, and sending the reader after a move
// that did not happen wastes their time. The ref/path split ambiguity splitHint
// exists for cannot arise here: a header records ref and path as separate keys,
// so nothing is being guessed.
func movedHint(src Source, err error) string {
	if !isNotFound(err) {
		return ""
	}
	return fmt.Sprintf(
		"\n\n%s/%s has no %q at %q. Either the model moved, in which case import it from its new address, or it was deleted, in which case this copy is orphaned and whether to keep it is your call",
		src.Owner, src.Repo, src.Path, src.Ref)
}

// newImports are the import paths the origin declares that the copy on disk did
// not. Vendoring fetches one file and resolution is not transitive (ADR-0015),
// so they are reported and never followed.
//
// Only the new ones, so a copy whose imports were understood at import time does
// not re-warn on every update. A copy that no longer parses locally is treated
// as having had none, which over-reports rather than staying quiet.
func newImports(local []byte, upstream *model.Model) []string {
	had := map[string]bool{}
	if m, err := model.Parse(local); err == nil {
		for _, imp := range m.Imports {
			had[imp.Path] = true
		}
	}
	var added []string
	for _, imp := range upstream.Imports {
		if !had[imp.Path] {
			added = append(added, imp.Path)
		}
	}
	return added
}
