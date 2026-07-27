// Package provenance reads and writes the comment header that marks a model
// file as a vendored copy of a model whose home is another repository.
package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// LinePrefix begins every provenance line.
//
// A line counts only when it starts at column zero. An indented "# modelith-"
// comment is an ordinary comment: it is neither parsed nor stripped, so the
// bytes Parse reads and the bytes Digest covers can never disagree about which
// lines are header. That single rule is the whole contract — the digest is
// defined by the strip (ADR-0015).
const LinePrefix = "# modelith-"

// lineRE matches one provenance line and splits it into key and value. The
// value is everything after the colon, trimmed; a free-text value such as the
// vendored banner is deliberately unconstrained.
var lineRE = regexp.MustCompile(`^# modelith-([a-z][a-z0-9-]*):(.*)$`)

// digestRE is the recorded digest's shape.
var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Banner is the value deps import writes for the vendored key. Nothing enforces
// it — a digest computed over the file cannot cover the header that records it —
// but an agent editing the file reads it and stops, which is the point.
const Banner = "DO NOT EDIT — this file is a copy. Change it at its origin."

// Header is the provenance block of a vendored model file.
type Header struct {
	Vendored string
	Fetch    string
	Origin   string
	Path     string
	Ref      string
	Commit   string
	Imported string
	Digest   string
}

// keyOrder is the order Format writes the keys in, and the set of keys that
// exist at all: a line naming anything else is a Problem.
var keyOrder = []string{"vendored", "fetch", "origin", "path", "ref", "commit", "imported", "digest"}

// commonKeys are required whatever the fetch method is. methodKeys are the ones
// each method requires on top, so adding a method means declaring what it
// carries rather than inheriting git's key set by default.
var (
	commonKeys = []string{"vendored", "fetch", "imported", "digest"}
	methodKeys = map[string][]string{
		"git": {"origin", "path", "ref", "commit"},
	}
)

// Methods returns the fetch methods this build understands.
func Methods() []string {
	// One entry today; sorted so a diagnostic listing them is deterministic
	// when there are more.
	out := make([]string, 0, len(methodKeys))
	for m := range methodKeys {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

func (h *Header) field(key string) *string {
	switch key {
	case "vendored":
		return &h.Vendored
	case "fetch":
		return &h.Fetch
	case "origin":
		return &h.Origin
	case "path":
		return &h.Path
	case "ref":
		return &h.Ref
	case "commit":
		return &h.Commit
	case "imported":
		return &h.Imported
	case "digest":
		return &h.Digest
	}
	return nil
}

// Problem is a defect in the header itself — an unknown key, a missing required
// one, a line outside the leading comment block. It carries the 1-based line so
// a diagnostic can point at it; Line is zero for a problem about the header as a
// whole rather than about one line.
type Problem struct {
	Line    int
	Message string
}

// Present reports whether src carries a provenance line at all.
//
// This, not a successful Parse, is what makes a file vendored. A header with a
// typo in it still means the file is somebody else's copy, and treating it as
// this repo's own work would bury the typo under a flood of completeness gaps
// about a document nobody here controls.
func Present(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		if lineRE.MatchString(line) {
			return true
		}
	}
	return false
}

// Parse reads the provenance header from src. It returns a nil Header when src
// carries no provenance line at all, which is the ordinary case for a model
// this repo owns. Otherwise it returns what it could read plus every defect it
// found; callers report the defects and may still use the header.
func Parse(src []byte) (*Header, []Problem) {
	if !Present(src) {
		return nil, nil
	}
	var (
		h        Header
		problems []Problem
		seen     = map[string]int{}
	)
	lines := strings.Split(string(src), "\n")
	lead := leadingCommentBlock(lines)
	for i, line := range lines {
		m := lineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		num, key, value := i+1, m[1], strings.TrimSpace(m[2])
		if i >= lead {
			problems = append(problems, Problem{num, fmt.Sprintf(
				"provenance line %q is below the leading comment block — the whole header belongs at the top of the file, before any model content",
				LinePrefix+key)})
			continue
		}
		field := h.field(key)
		if field == nil {
			problems = append(problems, Problem{num, fmt.Sprintf(
				"unknown provenance key %q — this modelith knows %s",
				LinePrefix+key, quotedList(keyOrder))})
			continue
		}
		if prev, dup := seen[key]; dup {
			problems = append(problems, Problem{num, fmt.Sprintf(
				"provenance key %q appears twice, on lines %d and %d", LinePrefix+key, prev, num)})
			continue
		}
		seen[key] = num
		*field = value
	}

	problems = append(problems, h.validate(seen)...)
	return &h, problems
}

// validate checks the key set against the fetch method and the shape of the
// values that have one. seen maps a key to the line it was read from, so a
// problem about a value points at it.
func (h *Header) validate(seen map[string]int) []Problem {
	var problems []Problem

	required := append([]string{}, commonKeys...)
	method, known := methodKeys[h.Fetch]
	switch {
	case h.Fetch == "":
		// The missing-key report below names it; naming a method that is not
		// there as unknown too would report one mistake twice.
	case !known:
		problems = append(problems, Problem{seen["fetch"], fmt.Sprintf(
			"unknown fetch method %q — this modelith supports %s. If you need another, please open an issue at https://github.com/stacklok/modelith/issues",
			h.Fetch, quotedList(Methods()))})
	default:
		required = append(required, method...)
	}

	for _, key := range required {
		if *h.field(key) == "" {
			problems = append(problems, Problem{0, fmt.Sprintf(
				"provenance header is missing %q", LinePrefix+key)})
		}
	}
	if h.Digest != "" && !digestRE.MatchString(h.Digest) {
		problems = append(problems, Problem{seen["digest"], fmt.Sprintf(
			"provenance digest %q is not in the form sha256:<64 hex digits>", h.Digest)})
	}
	return problems
}

// ValidDigest reports whether s is a digest in the form a header records. A
// caller that has already reported a malformed one uses this to skip the
// comparison, which such a digest cannot help but fail.
func ValidDigest(s string) bool { return digestRE.MatchString(s) }

// Verify reports whether src still hashes to the digest its own header records,
// and returns the digest src actually has.
func (h *Header) Verify(src []byte) (ok bool, got string) {
	got = Digest(src)
	return got == h.Digest, got
}

// Strip returns src with every provenance line removed, which is the content
// the digest covers.
func Strip(src []byte) []byte {
	lines := strings.Split(string(src), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if lineRE.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n"))
}

// Digest returns the digest of src as a header records it: SHA-256 over src
// with its provenance lines removed, so stamping a header does not change the
// digest of the file it is stamped into.
func Digest(src []byte) string {
	sum := sha256.Sum256(Strip(src))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Format renders h as the header block, every line terminated.
func (h *Header) Format() string {
	var b strings.Builder
	for _, key := range keyOrder {
		if v := *h.field(key); v != "" {
			fmt.Fprintf(&b, "%s%s: %s\n", LinePrefix, key, v)
		}
	}
	return b.String()
}

// Stamp returns src with h's header inserted: after a leading
// "# yaml-language-server:" line when src opens with one, so the editor
// directive stays first, and at the top otherwise.
func Stamp(src []byte, h *Header) []byte {
	lines := strings.Split(string(src), "\n")
	at := 0
	for i := 0; i < leadingCommentBlock(lines); i++ {
		if strings.HasPrefix(lines[i], "# yaml-language-server:") {
			at = i + 1
		}
	}
	head := strings.Join(lines[:at], "\n")
	if at > 0 {
		head += "\n"
	}
	return []byte(head + h.Format() + strings.Join(lines[at:], "\n"))
}

// leadingCommentBlock returns the number of lines in the run at the top of the
// file that are blank or comments — where the header belongs.
func leadingCommentBlock(lines []string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return i
	}
	return len(lines)
}

func quotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}
