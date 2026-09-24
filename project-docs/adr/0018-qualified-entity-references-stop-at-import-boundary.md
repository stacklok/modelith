# Qualified entity references stop at the import boundary

`relationship.entity` and `subtypeOf` may name a direct import's entity as `scope.Entity`. The importer validates that identity but does not traverse imported subtype ancestry, inherit imported invariants, or reconcile relationship reciprocity and ownership across the boundary. This keeps imported models independent of their importers while preserving explicit, offline direct-import resolution; speculative inherited-invariant behavior is tracked in #47 and qualified prose in #48.
