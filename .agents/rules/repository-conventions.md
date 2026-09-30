# Repository conventions

These rules extend the global working standards for this project.

## Public documentation

`docs/` is a public OKF bundle. Never add hostnames, absolute home paths,
tokens, internal project identifiers, or task ids. Use relative links only.

## Rendered output is generated

`.scratch/` and any `wiki/` directory are generated. Never hand-edit them, and
never commit them. Regenerate with `make render` or `make run`.

## Offline diagrams

The container must render Mermaid diagrams without network access. The vendor
bundle pins mermaid and the ELK layout in `mermaid/package.json`; keep those pins
together and rebuild with `make vendor` when they change. The CDN fallback in
`diagrams.js` names the same two exact versions, and a test asserts it, since a
moving tag there would mean a page draws a diagram one way offline and another
way online.

A render with a diagram and no vendor directory fails. That is deliberate: the
fallback reaches a public CDN from the reader's browser, so a silent return made
an offline deployment look fine while it was not. A bundle with no diagram needs
no runtime and still renders.

## Scope discipline

State the scope in one sentence before editing. Keep the diff inside that scope,
and prefer extending an existing construct over adding a parallel one.
