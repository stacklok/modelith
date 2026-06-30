# Contributing to modelith

Thanks for your interest in contributing! modelith is released under the
Apache 2.0 license. See the [README](./README.md) and [`docs/`](./docs/) for
what the tool does, and [`CLAUDE.md`](./CLAUDE.md) for repository layout and
conventions you should know before making structural changes (the example
fixture, schema/struct sync, and docs numbering rules all have CI checks
behind them).

## Code of conduct

This project adheres to the [Contributor Covenant](./CODE_OF_CONDUCT.md). By
participating, you're expected to uphold it. Report unacceptable behavior to
[code-of-conduct@stacklok.com](mailto:code-of-conduct@stacklok.com).

## Reporting security vulnerabilities

Please don't report security vulnerabilities using GitHub issues. Instead,
follow the process in [`SECURITY.md`](./SECURITY.md).

## How to contribute

Use [GitHub Issues](https://github.com/stacklok/modelith/issues) for bugs and
feature requests. For small, obvious fixes, feel free to just open a pull
request directly.

## Pull request process

- All commits must include a `Signed-off-by` trailer certifying the
  [Developer Certificate of Origin](./dco.md). Use `git commit -s`.
- Fork the repo, branch, and make your changes.
- Run `task check` before pushing — it runs the same checks as CI
  (vet, staticcheck, tests, model lint, render-check) plus local plugin
  validation. See [`docs/09-local-development.md`](./docs/09-local-development.md).
- Follow the commit message guidelines below.
- Open a PR; ensure CI passes.
- PRs are squash-merged.

## Commit message guidelines

We follow the conventions from
[Chris Beams' "How to Write a Git Commit Message"](https://chris.beams.io/posts/git-commit/):

1. Separate subject from body with a blank line
2. Limit the subject line to 50 characters
3. Capitalize the subject line
4. Do not end the subject line with a period
5. Use the imperative mood in the subject line
6. Use the body to explain what and why, not how
