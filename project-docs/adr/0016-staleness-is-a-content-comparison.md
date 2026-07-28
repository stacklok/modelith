# A vendored copy is stale when its content differs from its origin

`modelith deps check` decides staleness by comparing the origin's bytes against
the digest the header records, not by comparing commit SHAs. `# modelith-imported:`
dates the arrival of a version, not the last check, which is what forecloses a
"last checked" key. Extends ADR-0015 with the freshness half of the vendoring
lifecycle; nothing in ADR-0015 becomes untrue.

## Context

ADR-0015 settled how a vendored copy is verified *offline*: `lint` re-hashes the
file and compares it to `# modelith-digest:`, which answers "has my copy been
tampered with?". It left open the question the network can answer, "has the model
changed upstream?" — the job of `deps check` and `deps update`.

The header carries both a `commit` and a `digest`, so the obvious implementation
compares the recorded commit against the commit that last touches the path now.
`deps import` already fetches that SHA, so the machinery is in hand.

## Decision

**Staleness is a content comparison.** `check` fetches the file at the recorded
ref and compares `provenance.Digest` of those bytes against `# modelith-digest:`.

The comparison works because of a property of how `import` writes the header:
the digest is computed over the *upstream bytes before stamping*, and `Digest`
strips header lines, so what is recorded is the **origin's** content digest
rather than the local file's. Comparing it against the origin now is therefore
independent of whether anyone edited the local copy — which is what keeps
`check` and `lint` answering two genuinely different questions from one anchor.

Commit comparison is wrong in both directions. A merge commit or a
whitespace-only touch reports stale when nothing a reader would care about
moved, and a SHA that differs cannot say what differs. **The commit SHA stays in
the header and stays in the output as reporting**, resolved only when the
content has actually changed — so a clean check over N copies costs N `gh`
calls, not 2N.

**`# modelith-imported:` dates the content.** A copy whose origin has not moved
is left byte-for-byte alone by `deps update`, so the date records when *this
version arrived* rather than when the copy was last confirmed. That is the more
useful of the two meanings: `git blame` on the header line and the date agree,
and `modelith deps update *.modelith.yaml` becomes a habit with a clean diff.

The rejected alternative is a second key, `# modelith-checked:`. It would make
every check a write, which is exactly the property that makes `check` safe to
run in CI and safe to run against a read-only checkout.

**One write condition** follows from both, and no case needs a branch of its
own. `deps update` writes when `Digest(local) != Digest(upstream)`, or when the
ref changed. A hand-edited copy is therefore not current whatever upstream did,
and updating it restores it.

## Consequences

- **A pinned copy reports up to date forever.** A copy at `v2.1.0` is never
  stale by this test, even when `v2.3.0` has been out for months. `check` names
  the ref in every line of output so the verdict is visibly a statement about
  the pin rather than about the world. Chasing newer tags needs an answer to
  "which tags count as newer" — semver parsing, prereleases, projects that do
  not tag semantically — and modelith has no basis to decide that. Tracked as a
  follow-up.
- **Stale exits 1**, matching `render --check` drift. modelith has exactly one
  failure code and does not gain a second; a failure to check lands on the same
  code.
- **`deps update` overwrites local edits**, consistent with `deps import`, which
  already replaces an edited copy. These files are committed, so a discarded
  edit is recoverable from `git diff`; refusing would strand the user, whose
  only other remedy does the same thing.
- **ADR-0015 anticipated that `check` would diff the two files** so a human
  could judge a documentation-only change. It does not. `git diff` answers that
  question after an update with the user's own tooling, and reverting an update
  is one command. What is given up is seeing what moved *before* deciding to
  take it.
- Pinned by `TestADR_0016_StalenessIsContentNotCommit` and
  `TestADR_0016_ACurrentCopyIsNotRewritten`.
