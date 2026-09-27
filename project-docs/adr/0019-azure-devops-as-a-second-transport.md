# Azure DevOps is a second transport for vendoring

Vendoring can fetch from Azure DevOps (`dev.azure.com`) as well as GitHub,
delegating each host to its own CLI — `gh api` for GitHub, `az rest` for Azure
DevOps — and `deps check` / `deps update` work for both origins. Supersedes
ADR-0015's **`gh` is the only transport** decision; every other ADR-0015
decision stands.

## Context

ADR-0015 designated `gh` as the sole transport and made a non-GitHub origin an
error that asks the user to file an issue, on ADR-0007's bar that a second
transport waits for a real user. That user arrived: canonical domain models
hosted in private Azure DevOps Git repositories. ADR-0010 foresaw the slot —
`fetch: git` names what the origin *is*, not which binary fetched it — so no
header migration and no schema change are involved.

The hard constraint is ADR-0011: the binary holds no HTTP client, no TLS
configuration, and no credential handling; network work is delegated to an
external CLI, run as an argv array with no shell.

## Decision

**Delegate to `az rest`, not to an HTTP client, and not to `curl`.** Where
GitHub hands off to `gh api`, Azure DevOps hands off to `az rest`. `az`
resolves its own token from the user's `az login` session and performs the
request internally, so the binary never sees a credential. An earlier revision
of this work read a token from `az account get-access-token` and passed it to
`curl`; it was reverted, because routing a credential through the binary's
argument list breaks the no-credential property ADR-0011 keeps, and no `curl`
invocation — argv, stdin, or config file — restores it.

**The AAD audience is passed explicitly.** `az rest` cannot derive an Azure
DevOps audience from a `dev.azure.com` URL, so requests carry
`--resource 499b84ac-1321-427f-aa17-267ca6975798`, the well-known Microsoft
first-party application ID for Azure DevOps. That is a fact about the `az` CLI,
not configuration modelith invents.

**The Items API is asked for bytes.** The content fetch passes `download=true`.
Without it the endpoint returns a JSON `GitItem` describing the item rather
than its content, which the model parser then rejects; with it the body is the
file. Content is written to a temp file with `--output-file` and read back,
because `az rest` appends a newline when it prints a body to stdout — one byte
that would move the copy's digest off canonical. The bytes then match the origin
by construction rather than by trusting a printer.

**The header gains one optional key, `ref-type`, and an unknown type is
resolved rather than inferred.** An Azure DevOps version has a *type* — `GB` a
branch, `GT` a tag, `GC` a commit — and the API takes the type alongside the
value. The API does **not** infer a missing type: an untyped
`versionDescriptor.version` is read as a **branch**, so a copy pinned to a tag
or a commit cannot be fetched while the type is unset (verified against
api-version 7.1, 7.1-preview.1, and 7.2-preview.1: a 40-hex value with no type
fails with "The version descriptor <Branch: …> could not be resolved"). So an
ADO import records `# modelith-ref-type:` as `branch`, `tag`, or `commit`; the
key is optional and *omitted* for GitHub, whose API resolves an untyped ref on
its own, so a GitHub header written before the key existed is byte-identical to
one written now and needs no migration. When the type is not named — a bare
`version=`, a `--ref` override, or a header from before the key existed — it is
**resolved before the fetch**: a git object id (40 or 64 hex) is a commit, and
anything else is looked up with the refs API under `refs/heads/` and then
`refs/tags/`. A branch is preferred on a name collision, because that is what
the API itself defaults to and a tag is reachable from the URL's `GT` form. An
unknown value is an error, like an unknown `fetch:` method, which matches
ADR-0015's closed-set posture pre-release. There is deliberately no value
meaning "let the origin decide": the origin does not decide.

**Refresh is first-class for both hosts.** `deps check` and `deps update`
dispatch on the origin's host. A `github.com` origin is rebuilt into a blob URL
as before; a `dev.azure.com` origin is rebuilt into the typed ADO address from
`origin` (organization, project, repository), `path`, `ref`, and `ref-type`. The
content and commit fetchers dispatch the same way, so a copy from either host is
checked and updated rather than only imported. A `--ref` re-pin drops the
recorded type, which is then resolved afresh — a branch re-pinned to a tag must
not ask for a branch named after the tag, and an untyped request would be read
as one — and the header's `ref-type` is rewritten from what was resolved. A copy from a host with no transport is
still refused per file — a `Report`, not a run abort, so a mixed run still
judges its reachable copies — with an error naming the origin.

**A delegated CLI failure stops a batch, whoever the CLI is.** An absent or
unauthenticated `gh` or `az` fails every copy in a run identically, so the
"unusable" classification keys on both CLIs' own text (`gh auth login`,
`GH_TOKEN`, `az login`, `AADSTS`, HTTP 401) rather than on GitHub's alone.

**Process lifecycle is bounded on both platforms.** A delegated command that
spawns helpers inheriting its pipes can hold `Wait()` open past a context
deadline. On Unix the whole process group is killed (`Setpgid` plus a
negative-PID `SIGKILL`), with `cmd.WaitDelay` bounding the pipe drain if a
helper escapes the group; on Windows, which has no POSIX process groups, the
direct child is killed and `WaitDelay` still bounds the wait. A `--timeout`
decorator gives each delegated command its own deadline on `deps import`,
`deps check`, and `deps update`, and reports a fired deadline separately from a
caller's cancel (e.g. Ctrl+C). The Windows bound is soft by up to `WaitDelay`
(5s), which is an accepted platform cost rather than a documented flag
behaviour.

## Consequences

- `deps import`, `deps check`, and `deps update` accept `github.com` and
  `dev.azure.com` origins.
- `az` is a runtime prerequisite for Azure DevOps origins, the way `gh` is for
  GitHub; the "not installed" hint names the right CLI for the one that is
  missing.
- No schema change. `ref-type` is a header comment, not schema, so
  `TestSchemaStructSync` is not in play; `fetch: git` still records the same
  origin URL shape (`https://dev.azure.com/<org>/<project>/_git/<repo>`), so an
  ADO copy is verified offline against its digest exactly like a GitHub one.
- A legacy `*.visualstudio.com` URL is refused at parse time with a pointer to
  its `dev.azure.com` address, since the host moved.
- Pinned by `TestImport_ADO_StampsAVerifiableCopy` and the rest of the
  `TestImport_ADO_*` set (import end to end, the GB/GT/GC prefixes, the `--ref`
  override, and `download=true` on the items request);
  `TestADR_0019_RefTypeIsRecordedOnlyWhereItIsNeeded` (the `ref-type` key is
  recorded for ADO and omitted for GitHub); `TestRefresh_ADOCopyIsFirstClass`,
  `TestRefresh_ADORepinRetypesTheRef`,
  `TestRefresh_ADORepinToACommitTypesItAsACommit`,
  `TestRefresh_ADOHeaderWithoutRefTypeResolvesIt`, and
  `TestRefresh_ADOBareRefreshKeepsTheRecordedType` (refresh dispatch, the re-pin
  behaviour for a tag and for a commit, resolving a header without the key, and
  the recorded type); `TestRefresh_RefusesAnUnknownHost` (a
  host with no transport); `TestSurvey_ResolutionAbortsOnUnusableCLI` (the
  ref-type lookup stops the batch when the CLI is unusable);
  `TestExecRunner_KillsTheWholeProcessGroup` and
  `TestExecRunner_WaitDelayBoundsAPipeHeldByAStrayChild` (the process
  lifecycle); and the `TestTimeoutRunner_*` and
  `TestSurvey_TimeoutBoundsEachDelegatedCall` sets (the deadline).
