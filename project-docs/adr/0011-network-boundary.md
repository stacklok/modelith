# The network boundary is lint and render, not the whole binary

`lint`, `render`, and the packages behind them never perform network I/O.
Commands that do are named, opt-in, and delegate transport to an external tool
rather than implementing it. This replaces the blanket "no network calls" rule
in `go-style.md`, which was a proxy for the property actually wanted.

## Context

`go-style.md` says: *"If a feature seems to need a network call, a daemon, or a
browser, that's a sign to redesign it, not to add the dependency."* Written
when modelith only ever read local files, that rule cost nothing and protected
something real.

Cross-model references (ADR-0010) put pressure on it. A model vendored from
another repository has to get there somehow, and the honest options were to
abandon the feature, to make the user copy files by hand forever, or to admit
that a fetch is legitimate at a moment the user chooses. The blanket rule
could not express the third, so it would have been quietly violated or used to
block a feature it was never aimed at.

The property worth protecting was never "this binary contains no socket." It
is that the commands people run constantly — in editors, in pre-commit hooks,
in CI on every pull request — are fast, deterministic, and work on a plane.

## Decision

**`lint` and `render` never touch the network.** Neither do `internal/lint`,
`internal/render/...`, `internal/schema`, or `internal/model`. This holds no
matter what flags are passed; there is no escape hatch that makes lint fetch
something. A feature that would require it is redesigned or refused, exactly
as the old rule said.

**Commands that use the network say so in their name and are opt-in.** They
live under a distinct subcommand group, never run implicitly as a side effect
of another command, and never run at all unless asked. Where a question spans
both worlds — verifying a local file versus asking whether its origin moved —
the offline half belongs to the command people already run and the online half
is a separate command, not a flag on the first.

**modelith does not implement network transport.** It shells out to `git` and
`gh`, which already handle authentication, private hosts, proxies, SSH agents,
and 2FA. The binary keeps no HTTP client, no TLS configuration, and no
credential handling. This is the choice `cmd/go` made for VCS fetches.

## Why

- **Determinism where it is load-bearing.** Rendering is byte-deterministic and
  golden-tested. A renderer that can reach the network is a renderer whose
  output depends on someone else's uptime.
- **The common path stays offline.** Air-gapped work, flights, and CI that
  should not depend on a third party's availability all keep working, because
  the commands those settings run are the ones on the offline side.
- **No credential surface.** The class of bug where a tool mishandles a token
  cannot occur in a binary that has never seen one.
- **The old rule was a proxy.** "No network" was shorthand for "fast,
  deterministic, no runtime dependencies." Naming the property directly makes
  the rule usable in cases the shorthand mishandles.

## Consequences

- `go-style.md` is updated in this change to state the boundary rather than the
  prohibition, so the two do not disagree. That file remains authoritative on
  Go conventions per `CLAUDE.md`; an ADR records why a rule changed, it does not
  outrank the rule file. Keeping them in step matters because `go-style.md`
  loads on every Go edit and an ADR does not.
- A future proposal to make `lint` or `render` reach the network is refused by
  this ADR, whatever the convenience argument.
- Adding an HTTP client to any package remains off the table; a network-using
  command shells out instead.
- Pinned by `TestADR_0011_OfflinePackages`, which walks the transitive imports
  of the offline packages and fails if any of them reaches `net`, `net/http`,
  or `os/exec`. Banning `os/exec` there is deliberate: without it the rule
  could be evaded by shelling out to `curl`.
- That test guards the library packages, not the commands. `cmd/modelith`
  reaches `net` transitively through `spf13/pflag` today, and the network-using
  commands will legitimately import `os/exec`, so the package-level ban cannot
  extend there. "No flag makes `lint` fetch" is therefore a design commitment
  enforced by the library boundary rather than a claim a test proves end to
  end; a command-level guard would need to assert on wiring, not imports.
- The offline packages import `net/url` and `net/netip` today, via the JSON
  Schema library's format validation. Both are parsers that perform no I/O, so
  they are permitted by name.
