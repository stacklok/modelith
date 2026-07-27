package lint

import (
	"fmt"

	"github.com/stacklok/modelith/internal/provenance"
)

// runProvenance reports defects in the file's provenance header and verifies
// the copy against the digest that header records. It returns whether the file
// is vendored — a copy of a model whose home is another repository.
//
// A vendored file is not this repo's work, so the diagnostics that are findings
// about a model its authors control do not apply to it. What that suppresses is
// exactly two things, and no more (ADR-0015): the completeness category, in
// dropOwnedDiagnostics, and the errors its own imports list would raise, in
// loadImports. Structural and semantic checks still run: a vendored file that is
// not a valid domain model breaks this repo's build and is this repo's problem.
func runProvenance(src []byte, res *Result) bool {
	h, problems := provenance.Parse(src)
	if h == nil {
		return false
	}
	for _, p := range problems {
		msg := p.Message
		if p.Line > 0 {
			msg = fmt.Sprintf("line %d: %s", p.Line, msg)
		}
		res.Findings = append(res.Findings, Finding{
			Severity: SeverityError,
			Category: CategorySemantic,
			Path:     "",
			Message:  "provenance header: " + msg,
		})
	}
	// A digest that is not in the recorded shape is already reported above.
	// Comparing against it would report the same mistake a second time, as a
	// mismatch it cannot help but produce.
	if provenance.ValidDigest(h.Digest) {
		if ok, got := h.Verify(src); !ok {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityError,
				Category: CategorySemantic,
				Path:     "",
				Message: fmt.Sprintf(
					"this vendored file no longer matches the digest its provenance header records (recorded %s, computed %s) — it has been edited since it was imported. Refresh it with `modelith deps import %s`, or delete the provenance header if the change is a deliberate fork, which makes this repository the file's home",
					h.Digest, got, refreshTarget(h)),
			})
		}
	}
	return true
}

// refreshTarget is what to hand `modelith deps import` to replace this copy. For
// a git origin that is the blob URL the header's parts describe; for anything
// else the origin alone, which is all such a method records.
func refreshTarget(h *provenance.Header) string {
	if h.Fetch == "git" && h.Origin != "" && h.Ref != "" && h.Path != "" {
		return fmt.Sprintf("%s/blob/%s/%s", h.Origin, h.Ref, h.Path)
	}
	return h.Origin
}

// dropOwnedDiagnostics removes the findings that are about a model's own
// authoring rather than about this repository's use of it. They are exactly the
// completeness category: entities with no invariants, entities no scenario
// exercises, unused glossary terms, unused enums, and an import nothing
// references.
//
// Without this the Action this repo ships — which lints every globbed file with
// --completeness — turns a user's CI red for gaps in somebody else's document.
func dropOwnedDiagnostics(res *Result) {
	kept := res.Findings[:0]
	for _, f := range res.Findings {
		if f.Category == CategoryCompleteness {
			continue
		}
		kept = append(kept, f)
	}
	res.Findings = kept
}
