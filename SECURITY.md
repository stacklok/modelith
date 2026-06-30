# Security Policy

We take security seriously and appreciate responsible disclosure of any
vulnerabilities found in modelith.

## Reporting a vulnerability

Please report security issues using the GitHub Security Advisory
["Report a Vulnerability"](https://github.com/stacklok/modelith/security/advisories/new)
tab on this repository.

If you can't use GitHub, email [security@stacklok.com](mailto:security@stacklok.com)
instead.

Please don't report security vulnerabilities using public GitHub issues.

Include as much detail as you can: steps to reproduce, the affected version(s),
and any sample files needed to reproduce the issue.

## What to expect

- We'll acknowledge your report within a few business days.
- We'll work with you privately to understand and fix the issue before any
  public disclosure.
- Once a fix is released, we'll publish a GitHub Security Advisory describing
  the issue and crediting the reporter, unless you'd prefer to stay anonymous.

modelith is a local linting/rendering CLI with no network service and no
runtime secret handling, so most realistic findings will be things like
parser crashes on malformed input rather than remote compromise — but we'd
still rather hear about it privately first.

## Disclosure

We ask that vulnerabilities be handled under a
[responsible disclosure](https://en.wikipedia.org/wiki/Responsible_disclosure)
model: please give us a reasonable window to investigate and ship a fix before
making anything public.
