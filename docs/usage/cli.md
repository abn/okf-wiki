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

# the same two paths positionally, content then output
okf-wiki render ./docs ./public
```

`--content` is optional. When it is omitted, okf-wiki looks in `/content`,
`/docs`, `/wiki-content`, the working directory, then the container's bind
mounts, and uses the first directory that looks like an OKF bundle.

`render` and `serve` also take the content directory and the output directory as
positional arguments, in that order, and they may be mixed with flags in any
order. A flag or an environment variable still wins: a positional argument fills
in only what was not stated explicitly.

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

## Live reload

`serve --watch` re-renders when the bundle changes, so an edit is live on the
next page load and nothing is restarted:

```bash
okf-wiki serve --content ./docs --out .scratch/wiki --watch
```

It polls the bundle once a second, and the theme when one is layered over
the default, and re-renders into the same output directory the server reads
from. A re-render that fails, which is what saving part way through an edit
looks like, is reported and the last good render keeps serving until the
next change, so the server does not have to be restarted by hand.
`OKF_WIKI_WATCH=true` is the same thing for a container.

Every change rebuilds the whole site, not only the page that moved. That is
deliberate and cheap at a bundle's scale; [ADR 0004](../adr/0004-watch-rerenders-the-whole-tree.md) records why, and the measured cost.

`--watch-interval` changes how often the bundle is polled, as a Go duration
(`--watch-interval 2s`, or `OKF_WIKI_WATCH_INTERVAL`). It is how soon an edit
appears, and how often a stream of changes re-renders; the default is one
second. See [`reference/configuration.md`](../reference/configuration.md).

The render is swapped into place rather than written over the output, so a page
loaded while a change re-renders never sees a half-written site or a missing
file.

## Multi-site trees

`--multi-site` reads the content directory as a tree of wikis rather than one
bundle. A directory holding an `index.md` is a wiki, rendered at the path it
sits at under `--base`; every other directory is a group, given a generated
index of the wikis and groups directly inside it. A wiki's own sub-directories
are its sections, so the tree stops at a wiki.

```text
content/
  clients/         a group, named by its .meta.json
    sprind/        a group
      operations/  a wiki: an index.md at its root
      strategy/    a wiki
  homelab/         a wiki at the top level
```

Rendered with `--multi-site --base /`, that publishes `/` and `/clients/` as the
group indexes, `/homelab/` as a wiki, and the two SPRIND wikis under
`/clients/sprind/`. The layout on disk mirrors the tree, so a `render` and a
`serve` publish the same thing, and `--watch` re-renders the whole tree on a
change. A group takes its name and summary from a `.meta.json`
(`{"title": ..., "description": ...}`) or a `README.md` in that directory, a
wiki's from its own frontmatter, and a directory with neither from its name.

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
`--base /docs/deep/`. That directory holds the pages, each page's Markdown
source beside it for download, the resolved theme's assets,
`search-index.json`, and a `theme.json` recording which theme produced it. The
output is a plain static site, so any static file server can host it.

## Local development

```bash
make run      # vendor the diagram bundle, render docs/, serve on :8080
make render   # render only
make render-themed THEME=./themes/acme   # the same, with a theme layered over
```
