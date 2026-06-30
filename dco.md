# Developer Certificate of Origin

modelith requires every commit in a pull request to be signed off, certifying
that you wrote it or otherwise have the right to submit it under the project's
license.

## Developer Certificate of Origin 1.1

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

## How to sign off

Add a `Signed-off-by` trailer to your commit message with your real name and
email address:

```
Signed-off-by: Jane Doe <jane@example.com>
```

Git can add this for you automatically with the `-s` flag:

```sh
git commit -s -m "Fix the thing"
```

If you forgot to sign off an existing commit:

```sh
git commit --amend -s
```

To sign off multiple commits on a branch at once:

```sh
git rebase --signoff main
```

CI checks every commit in a pull request for a valid `Signed-off-by` trailer
and will fail the build if one is missing.
