---
type: Guide
title: Command line
description: Running okf-wiki as a binary, with render, serve, and detect.
tags: [usage, cli]
---

# Command line

```bash
# render a bundle to a static directory
okf-wiki render --content ./docs --out ./public

# render then serve until interrupted
okf-wiki serve --content ./docs --out .scratch/wiki --addr 127.0.0.1:8080 --open

# print the auto-detected content directory
okf-wiki detect
```

`--content` is optional. When it is omitted, okf-wiki looks in `/content`,
`/docs`, `/wiki-content`, the working directory, then the container's bind
mounts, and uses the first directory that looks like an OKF bundle.

## Theming and placement

```bash
# layer a theme over the embedded default
okf-wiki render --content ./docs --out ./public --theme ./themes/acme

# serve the wiki from a different path
okf-wiki serve --content ./docs --out ./public --base /docs/deep/
```

`--theme` takes a directory holding a `theme.json` and layers it over the theme
compiled into the binary, so it only has to carry what it changes. Both flags
have environment equivalents, `OKF_WIKI_THEME` and `OKF_WIKI_BASE`. See
[`reference/themes.md`](../reference/themes.md).

`--base` is both the URL prefix and the directory the pages are written into
under `--out`, so the rendered directory can be served by any static file server
unchanged. The default is `/wiki/`.

## Installing it

There are no tagged releases, so there is no release tarball to download and no
`brew`, RPM or deb package. Two paths work, and neither is a versioned release.

**The container**, which is the supported one and the reason there is nothing
else. It is published on every push to `main`, multiarch, so the same reference
runs on amd64 and arm64:

```bash
podman run --rm --network=host \
  -v "$(pwd)/docs:/content:ro,Z" ghcr.io/abn/okf-wiki:latest
```

**`go install`**, which works, with a caveat worth knowing before you rely on it:

```bash
go install github.com/abn/okf-wiki/cmd/okf-wiki@latest
```

With no tags, `@latest` resolves to a pseudo-version pinned to whatever `main`
happened to be, so it prints something like
`v0.0.0-20260929163058-1f172e4994c7` and two people running that command minutes
apart can get different binaries. It is a moving target, not a release. For
anything reproducible, pin a commit or build from a checkout you chose.

A related consequence: `okf-wiki version` prints a constant compiled into the
binary, not a version derived from git. It reads `0.1.0`, it is not kept in step
with anything, and the build passes no `-X` ldflag to override it. Treat it as a
label rather than a version, and read the pseudo-version Go resolved for the
provenance of a `go install` build.

## Reading these docs

This bundle is published to Pages at
<https://abn.github.io/okf-wiki/>, rendered from `main` by the repository's own
action. If you are reading this page locally, you are reading whatever `docs/`
holds in your checkout, which may be ahead of or behind what is published.

## Rendered output

`--out` receives `index.html`, a redirect to the wiki, and the wiki itself at
the base path: `out/wiki/` by default, and `out/docs/deep/` for
`--base /docs/deep/`. That directory holds the pages, the resolved theme's
assets, `search-index.json`, and a `theme.json` recording which theme produced
it. The output is a plain static site, so any static file server can host it.

## Local development

```bash
make run      # vendor the diagram bundle, render docs/, serve on :8080
make render   # render only
make render-themed THEME=./themes/acme   # the same, with a theme layered over
```
