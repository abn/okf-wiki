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

# the same, offering the whole source bundle as one download
okf-wiki render --content ./docs --out ./public --bundle

# render then serve until interrupted
okf-wiki serve --content ./docs --out .scratch/wiki --addr 127.0.0.1:8080 --open

# the same server, also exposing the read-only MCP endpoint at /wiki/_/mcp
okf-wiki serve --content ./docs --out .scratch/wiki --mcp

# search a rendered site
okf-wiki search "graph" --out .scratch/wiki --base /wiki/ --json

# serve search over MCP stdio, locally or from published wikis
okf-wiki mcp --out .scratch/wiki --base /wiki/
okf-wiki mcp --source docs=https://docs.example.com/wiki/ --source-token docs=DOCS_TOKEN
okf-wiki mcp --http 127.0.0.1:8090 --source docs=https://docs.example.com/wiki/

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

## Downloading the bundle

`--bundle` writes the source bundle beside the rendered pages as
`okf-bundle.zip` and adds a download control to every page. It is off by
default, because the archive is a second copy of the content in the output. The
environment equivalent is `OKF_WIKI_BUNDLE=true`. See
[Download the bundle](../design/rendering.md#download-the-bundle).

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

## MCP beside the wiki

`serve --mcp` mounts the read-only MCP server on the wiki's own listener, at
`<base>/_/mcp`:

```bash
okf-wiki serve --content ./docs --mcp
# MCP endpoint: http://127.0.0.1:8080/wiki/_/mcp

# skip the _ segment and mount at /wiki/mcp
okf-wiki serve --content ./docs --mcp --mcp-prefix ""
```

The `_/` segment keeps the endpoint outside the page namespace, so a page or an
asset named `mcp` cannot shadow it, and one proxy rule for the base publishes
both the wiki and the endpoint. With the default prefix, `<base>/_/` is
reserved for server endpoints. Set `--mcp-prefix ""` to mount at `<base>/mcp`
instead, or name a single segment of letters, digits, dot, dash or underscore,
such as `api`, for `<base>/api/mcp`. The endpoint path wins over a section at
the same URL, so a prefix can shadow a section directory of that name. It reads
the render `serve` just produced, and reloads before each tool call, so it
follows `--watch`. Like the wiki, it has no authentication and binds loopback by
default. `OKF_WIKI_MCP=true` is the container equivalent. See
[MCP server](mcp.md).

## Multi-site trees

`--multi-site` reads the content directory as a tree of wikis rather than one
bundle. A directory holding an `index.md` is a wiki, rendered at the path it
sits at under `--base`; every other directory is a group, given a generated
index of the wikis and groups directly inside it. A wiki's own sub-directories
are its sections, so the tree stops at a wiki.

```text
content/
  clients/         a group, named by its .meta.json
    example/       a group
      operations/  a wiki: an index.md at its root
      strategy/    a wiki
  homelab/         a wiki at the top level
```

Rendered with `--multi-site --base /`, that publishes `/` and `/clients/` as the
group indexes, `/homelab/` as a wiki, and the two example wikis under
`/clients/example/`. The layout on disk mirrors the tree, so a `render` and a
`serve` publish the same thing, and `--watch` re-renders the whole tree on a
change. A group takes its name and summary from a `.meta.json`
(`{"title": ..., "description": ...}`) or a `README.md` in that directory, a
wiki's from its own frontmatter, and a directory with neither from its name.

## The graph view

Every render writes `graph.html` at the base, and the graph control in the
header opens it. The page fetches `graph.json` when it opens, so a reader who
never opens it does not download the catalogue. In a multi-site tree the one
page at the tree base draws the aggregate, and every wiki links to it.

`--graph-inline` embeds the catalogue in the page instead. The page is larger
and needs no second request, which is useful when a viewer blocks `fetch` of a
sibling file. `OKF_WIKI_GRAPH_INLINE=true` is the same thing.

The view reserves `graph.html` at the render root. A page `graph.md` or a static
file of that name there fails the render with the path named, rather than being
overwritten. See [Graph view](graph.md).

## Offline folder

`--offline` writes a folder that opens from `file://`: relative URLs, a per-page
`<base>`, home links that name `index.html`, script-loaded search data, an
embedded graph, and a classic Mermaid bundle for pages with diagrams. No request
leaves the folder. `--repo` is refused, since a repository mount needs a server.

```bash
okf-wiki render --content ./docs --out ./site --offline
```

Open `site/wiki/index.html` or `site/index.html`. The folder can be moved as a
unit. See [Offline folder](offline.md).

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

`search` reads a rendered site rather than the source bundle. `mcp` reads either
a rendered output or one or more published wiki base URLs, so it can act as a
standalone federator, while `serve --mcp` reads the render it just produced on
the wiki's own listener. See [MCP server](mcp.md) for local, remote, federated
and alongside-serve examples, and [the reference](../reference/agent-access.md)
for the result contract and tool schemas.

`--out` receives `index.html`, a redirect to the wiki, and the wiki itself at
the base path: `out/wiki/` by default, and `out/docs/deep/` for
`--base /docs/deep/`. That directory holds the pages, each page's Markdown
source beside it for download, the resolved theme's assets (including the
search, diagram and graph clients), `search-index.json` and its text companion
`search-content.json`, the structural `graph.json`, the `graph.html` view,
`llms.txt` and `pages.json` for agent discovery, `changes.json` and
`health.json` for change tracking and graph health, `okf-bundle.zip` when
`--bundle` is set, and a `theme.json` recording which theme produced it. An
offline render also writes the search and tag data as `.js` scripts, and copies
the Mermaid runtime build it needs when the bundle has a diagram. The output is
a plain static site, so any static file server can host it, and an offline
folder can be opened from disk.

## Local development

```bash
make run      # vendor the diagram bundle, render docs/, serve on :8080
make render   # render only
make render-themed THEME=./themes/acme   # the same, with a theme layered over
```
