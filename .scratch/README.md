# .scratch

Repo-local, gitignored scratch space. Spikes, smoke tests, throwaway fixtures,
review round-records, profiles, and any working file that would otherwise land
in `/tmp` or a session temp dir go here.

Why in the repo instead of `/tmp`: it stays browsable in your editor and
survives the session. It is **not** committed — everything here except this
README is gitignored. Clean it out manually when it goes stale.

Durable records do not belong here. Decisions go to
[`../project-docs/adr/`](../project-docs/adr/), audit runs to
[`../project-docs/audits/`](../project-docs/audits/).
