# Conventions for these docs

> This file is **not published**. Docusaurus excludes `_`-prefixed Markdown, so
> it stays a contributor note here and on GitHub but never renders on the site.

These docs are built with Docusaurus from the `website/` directory and published
at **<https://modelith.sh>**. The `docs/` directory here is the content source;
`website/docusaurus.config.ts` points at it via `docsPath: '../docs'`.

## File order and naming

- **Number page files `NN-name.md`** (`02-getting-started.md`, …,
  `09-local-development.md`) so they read in order when browsed on GitHub.
  Docusaurus strips the `NN-` prefix from the URL and uses it for sidebar order,
  so the published slug is clean (`…/getting-started`).
- **The landing page is plain `index.md` — never numbered.** Docusaurus treats
  `index.md` as the folder root.
- Renaming or renumbering a page **changes its published URL.** Update any
  inbound links (README, other docs pages) when you do.

## Front matter

- Set **`title:`** (controls the page title and sidebar label).
- **`sidebar_position:`** is fine and harmless alongside the number prefix.
- **`slug:`** is safe to use — we control the base URL, so absolute slugs work.

## Linking and Markdown

- **Link between docs with relative paths that include the number prefix** —
  `[x](./07-cli.md)`, `[y](../03-understanding-your-model.md)`. Relative links
  are portable and survive any future site restructuring.
- **No HTML comments** (`<!-- -->`) — the site is MDX and rejects them. Use
  `{/* */}` instead.

## CI coupling — the parking-garage example

The worked example lives in **`docs/05-parking-garage/`**, and its
`garage.modelith.yaml` is linted and render-checked in CI alongside the
`examples/`. Two files glob it **by path**:

- `Taskfile.yml` (the `EXAMPLES` var)
- `.github/workflows/ci.yml`

If you renumber or rename that directory, **update both globs** or `task
lint-models` / `render-check` (and CI) will silently stop checking it. After any
change to the example or the renderer, run `task render` to regenerate
`garage.modelith.md` — `render-check` fails on drift.
