package lint

import (
	"os"
	"path/filepath"
	"strings"
)

// maxSymlinkHops bounds the link-following in realPath. filepath.EvalSymlinks
// does its own bounding; this covers the links it refuses to follow because
// their target does not exist, where a loop would otherwise spin forever.
const maxSymlinkHops = 40

// resolutionRoot returns the directory an import may not resolve outside of,
// and whether an enclosing repository defined it. The root is the nearest
// ancestor of the model holding a `.git` entry; failing that, the model's own
// directory, because outside a repository the tool cannot know the project's
// extent and assumes the smallest safe thing (ADR-0013).
//
// The `.git` entry is tested for existence, not for being a directory: in a
// linked worktree and in a submodule it is a regular file holding a gitdir
// pointer, and reading that as "no repository" would confine resolution to a
// single directory in every such checkout.
func resolutionRoot(modelPath string) (root string, inRepo bool) {
	dir := absolute(filepath.Dir(modelPath))
	for cur := dir; ; {
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			return realPath(cur), true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return realPath(dir), false
		}
		cur = parent
	}
}

// withinRoot reports whether candidate sits at or below root. Both must already
// be absolute, cleaned and symlink-resolved. filepath.Rel decides it rather
// than a string prefix, which would place "/repo-evil" inside "/repo".
func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// realPath resolves p as far as the filesystem allows. A component that does
// not exist is kept as written, because a missing import is reported as
// unreadable rather than as an escape. A dangling symlink is followed via its
// recorded target, so a link is judged by where it points and not by where the
// link file sits — otherwise a link committed inside a repository aims anywhere
// and the boundary is decorative.
func realPath(p string) string {
	p = filepath.Clean(p)
	var tail []string
	for hops := 0; hops <= maxSymlinkHops; {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, filepath.Join(tail...))
		}
		// EvalSymlinks gives up on a link whose target is missing. Follow it by
		// hand so the target, not the link's own location, is what gets judged.
		if target, err := os.Readlink(p); err == nil {
			hops++
			if filepath.IsAbs(target) {
				p = filepath.Clean(target)
			} else {
				p = filepath.Clean(filepath.Join(filepath.Dir(p), target))
			}
			continue
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		tail = append([]string{filepath.Base(p)}, tail...)
		p = parent
	}
	return filepath.Join(p, filepath.Join(tail...))
}

// absolute makes p absolute against the working directory, falling back to the
// cleaned path in the one case filepath.Abs fails — the working directory being
// unavailable, where there is nothing better to say.
func absolute(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}
