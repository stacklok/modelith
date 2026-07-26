# Import resolution is confined to the enclosing repository

An `imports:` entry may only name a file inside the repository holding the model
being linted — the nearest ancestor with a `.git` entry. Outside a repository
the root is the model's own directory. An import resolving beyond the root is an
error, decided before the file is opened.

## Context

Paths in `imports:` are relative, and `..` has to work: peer models commonly sit
in sibling directories, and the worked example relies on it (ADR-0010). Nothing
bounded how far `..` could walk, so `../../../../etc/passwd` was read.

Reading is the leak. An import produces four distinguishable diagnostics — it
resolves, it is missing, it is unreadable, it holds no model — and that is an
existence-and-permission oracle over the whole filesystem. modelith ships a
GitHub Action that lints models in CI, so a `*.modelith.yaml` from a fork can
probe the runner and report what it finds back through diagnostic text, which
lands in a pull-request check anyone can read.

## Decision

**The resolution root is the nearest ancestor of the linted model containing a
`.git` entry.** An import that resolves outside it is a semantic error naming
the offending path, where it resolved to, and the root.

**With no `.git` anywhere above, the root is the directory holding the model.**
Outside a repository the tool cannot know the project's extent, so it assumes the
smallest safe thing. This is the surprising case, so its diagnostic says why the
root is so tight.

**The root is computed per model, from that model's own location.** Linting two
models in different repositories in one invocation gives each its own root. Only
the direct imports of the model being linted are checked, because resolution does
not recurse (ADR-0010); a glob that lints an imported file directly gives that
file its own root.

Three details decide whether this works at all, and each is pinned by a case in
`TestADR_0013_ImportsConfinedToTheRepository`:

- **`.git` is tested for existence, not for being a directory.** In a linked
  worktree and in a submodule it is a regular file holding a `gitdir:` pointer.
  Requiring a directory would collapse the root to a single directory in every
  such checkout.
- **Symlinks are resolved before the containment test**, including a dangling
  one, which is followed via its recorded target. A link is judged by where it
  points, not by where the link file sits; otherwise a link committed inside the
  repository aims anywhere and the boundary is decorative. A dangling link left
  unresolved would also keep the oracle alive, since "unreadable" is one of the
  four answers.
- **Containment is `filepath.Rel`, not `strings.HasPrefix`.** A string prefix
  places `/repo-evil` inside `/repo`.

## Considered options

**A flag naming the root.** Rejected for now, not forever: no real case needs one
yet, and the fallback covers the loose-file case. Because it does not exist, no
diagnostic may advise passing one — an error that recommends a flag the binary
does not have is worse than the confinement it explains.

**Refusing `..` outright.** Simple and airtight, but it breaks the peer-directory
layout the format is designed around and the worked example already uses.
Bounding the walk keeps the layout and removes the oracle.

## Consequences

- A repository that vendors models above its own root can no longer import them.
  That is the intended loss; vendoring inside the repository is what ADR-0010
  describes.
- The oracle is closed across the repository boundary, not within it. An attacker
  who can already place a file in your repository has better options than a lint
  diagnostic, so probing inside the root is accepted rather than engineered
  against. The docs say so.
- The root walk touches the real filesystem, so it does not go through lint's
  `FileReader` seam. Tests that need a real tree use `t.TempDir()`; the seam
  still covers the reads.
- The error text names absolute paths — the resolved candidate and the root.
  This is the same disclosure the pre-existing "cannot be read" diagnostic
  already makes, and without it the message cannot explain a `..` chain or a
  symlink.
