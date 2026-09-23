# Recursive provenance verification follows locally readable vendored imports

`modelith lint` follows locally readable import edges while verifying provenance,
including edges declared by vendored models. This narrowly supersedes only
ADR-0015's consequence that suppressing a vendored model's import diagnostics
prevents traversal of those imports; all other ADR-0015 decisions stand.

## Decision

The crawl stays offline and integrity-only. It verifies every readable vendored
copy it reaches against the digest in that copy's header, reporting an actual
copied-file digest mismatch against that file. Missing, broken, unreadable, or
otherwise unusable nested edges remain silent, and the crawl does not perform
transitive semantic lint or network access. Normal import resolution remains
non-transitive: an importer still binds only the model it names directly.

The additional local walk catches a hand-edited vendored grandchild that was
otherwise hidden behind a vendored intermediary, without taking ownership of
that intermediary's model semantics or fetching a dependency tree. It is pinned
by `TestADR_0017_VendoredIntermediaryReachesMismatchedVendoredGrandchild`.
