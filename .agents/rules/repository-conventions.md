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
together and rebuild with `make vendor` when they change. The runtime falls back
to the CDN only when the vendor bundle is absent.

## Scope discipline

State the scope in one sentence before editing. Keep the diff inside that scope,
and prefer extending an existing construct over adding a parallel one.
