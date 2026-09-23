# A header digest mismatch requires update

A local copy can match its origin while `# modelith-digest:` still describes older bytes, such as after a merge keeps an older header. `deps update` writes when `Digest(local) != Header.Digest`, as well as when local and upstream differ or the ref changes, so it repairs the header and `lint` accepts the copy again. This supersedes ADR-0016's relevant write-condition decision. Pinned by `TestADR_0017_HeaderDigestMismatchRequiresUpdate`.
