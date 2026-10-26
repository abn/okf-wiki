---
type: Reference
title: Configuration
description: Every flag and environment variable, with defaults.
tags: [reference, configuration]
---

# Configuration

Resolution order is flag, then environment variable, then default.

| Flag | Environment | Default | Purpose |
| :--- | :--- | :--- | :--- |
| `--content` | `OKF_WIKI_CONTENT` | auto-detect | OKF bundle directory, or a tree of them with `--multi-site` |
| `--multi-site` | `OKF_WIKI_MULTI_SITE` | `false` | Treat the content directory as a tree of wikis |
| `--graph-inline` | `OKF_WIKI_GRAPH_INLINE` | `false` | Embed the graph catalogue in the graph page, with no fetch |
| `--offline` | `OKF_WIKI_OFFLINE` | `false` | Write a portable folder for file://; implies `--graph-inline` |
| `--bundle` | `OKF_WIKI_BUNDLE` | `false` | Offer the source bundle as `okf-bundle.zip` |
| `--out` | `OKF_WIKI_OUT` | `.scratch/wiki` | Rendered output root |
| `--base` | `OKF_WIKI_BASE` | `/wiki/` | URL prefix, and the path inside the output root |
| `--lang` | `OKF_WIKI_LANG` | `en` | Language the pages declare, a tag such as `de` or `pt-BR` |
| `--repo` | `OKF_WIKI_REPO` | empty | Repository root served at `/repo/` |
| `--vendor` | `OKF_WIKI_VENDOR` | `mermaid` | Directory holding the mermaid ESM and classic bundles. Required when the bundle has a diagram |
| `--theme` | `OKF_WIKI_THEME` | empty | Theme directory layered over the embedded default |
| `--brand` | `OKF_WIKI_BRAND` | `okf-wiki` | Wordmark |
| `--brand-sub` | `OKF_WIKI_BRAND_SUB` | `docs` | Subtitle label |
| `--version-tag` | `OKF_WIKI_VERSION_TAG` | empty | Freeform version label shown beside the wordmark |
| `--title` | `OKF_WIKI_TITLE` | `Wiki` | Document title suffix |
| `--sections` | `OKF_WIKI_SECTIONS` | empty | Section order, `id:Title` pairs |
| `--nav-links` | `OKF_WIKI_NAV_LINKS` | empty | Header links, `Label=URL` pairs |
| `--addr` | `OKF_WIKI_ADDR` | `127.0.0.1:8080` | Listen address (serve only) |
| `--open` | `OKF_WIKI_OPEN` | `false` | Open a browser (serve only) |
| `--watch` | `OKF_WIKI_WATCH` | `false` | Re-render when the bundle or theme changes (serve only) |
| `--watch-interval` | `OKF_WIKI_WATCH_INTERVAL` | `1s` | How often `--watch` polls for changes (serve only) |
| `--mcp` | `OKF_WIKI_MCP` | `false` | Serve the read-only MCP endpoint at `<base>/_/mcp` (serve only) |

`--multi-site` turns the content directory from one bundle into a tree of
them. A directory holding an `index.md` is a wiki and is rendered at the path
it sits at under the base; every other directory is a group and is given a
generated index of the wikis and groups directly inside it. The output mirrors
the tree, so one render publishes the whole collection and one `serve` serves
it from one port.

`--graph-inline` embeds the graph catalogue in `graph.html` instead of leaving
the client to fetch `graph.json`. The view then makes no second request, at the
cost of a larger page. It is the same switch for a single bundle and a tree; a
tree embeds its aggregate once at the base. See
[Graph view](../usage/graph.md).

`--offline` writes a portable folder for opening from `file://`. It implies
`--graph-inline`; it also writes relative URLs behind a per-page `<base>`, names
`index.html` for home links, rewrites same-page fragments to name their page,
writes the search and tag data as classic scripts, and copies a classic Mermaid
bundle for pages with diagrams. A diagram needs `vendor/mermaid-bundle.min.js`
from `make vendor`; the render fails if it is missing. It refuses `--repo`,
because a repository mount is a server endpoint. `--base` still names the
directory inside `--out`; it is not used as an absolute URL prefix. See
[Offline folder](../usage/offline.md).

`--bundle` writes `okf-bundle.zip` at each wiki's base and offers it from the
pages. It is off by default: the archive is a second copy of the content in the
output, so a deployment opts in when it wants the whole source downloadable in
one file. The path is reserved while it is on, and a bundle that already holds
that file fails the render rather than losing it. See
[Download the bundle](../design/rendering.md#download-the-bundle).

`--base` is both the URL prefix and the directory the pages are written into
under `--out`, so a rendered directory can be served by any static file server
unchanged. In offline mode it is only the directory; the URLs are relative. It
must not contain `.` or `..` segments.

A base of `/` is the whole site: the pages are written at the top of `--out`
with no wiki subdirectory, and there is no root redirect, because the wiki's own
`index.html` already occupies that path. This is the base the GitHub Action
computes for a user or organisation Pages site, so it is a normal configuration
rather than an edge case.

`--lang` is the language the pages declare on the `<html>` element, for a screen
reader and a read-aloud voice: a subtag such as `de`, or one with a region such
as `pt-BR`. A value carrying anything else is refused, because a browser ignores
a tag it cannot read and would then read every page as English whatever was
configured. The root redirect carries the same language as the pages it
redirects to.

`--nav-links` adds links to the header, comma-separated as `Label=URL` pairs. A
target with an `http` or `https` scheme leaves the wiki: it opens in a new tab
and is marked with an arrow. Any other target is a path in this bundle and
resolves exactly as a body link does, from the bundle root, so
`Usage=usage/cli.md` becomes the rendered page under the base and opens in the
same tab. A pair missing its label or its target is skipped. On a phone the
links move into the navigation drawer, the in-wiki ones under "On this wiki" and
the rest under "Elsewhere".

`--watch-interval` sets how often `serve --watch` polls for changes, as a Go
duration (`500ms`, `2s`). It trades how soon an edit appears against how much
work a page is doing: a change is noticed within one interval, and a stream of
changes renders about once per interval rather than once per change, so a longer
interval is quieter on a large tree. The default of one second suits authoring.

`--mcp` adds the read-only MCP endpoint to the wiki's listener at
`<base>/_/mcp`, so one process and one port publish the pages and the agent
surface. The `_/` segment is reserved for server endpoints, outside the page
namespace. The endpoint has no authentication and no TLS, like the wiki beside
it; it follows `--addr` and is loopback by default. See
[MCP server](../usage/mcp.md).

## Endpoints

- `/wiki/` and `/wiki/<slug>.html`: the rendered wiki, under `--base`.
  `/wiki/graph.html` is the graph view, and it fetches `/wiki/graph.json`
  unless the catalogue was inlined.
- `/wiki/okf-bundle.zip`: with `--bundle`, the wiki's source bundle, Markdown
  and assets under one folder. A multi-site tree writes one per wiki at that
  wiki's base.
- `/wiki/_/mcp`: read-only MCP endpoint, when `--mcp` is set. It sits under
  `--base`, so a base of `/docs/` puts it at `/docs/_/mcp`, and a base of `/`
  puts it at `/_/mcp`.
- `/repo/<path>`: read-only repository mount, when `--repo` is set. Dotfiles are
  refused.
- `/healthz`: liveness probe.
- `/`: redirects to `--base`, unless `--base` is `/`, where it is the wiki
  itself.

## Shutdown

`serve` handles `SIGINT` and `SIGTERM` and drains before exiting, so a request
in flight finishes rather than being cut mid-body. That is the container's stop
path, and a truncated page body is worse than a slightly slower stop. The drain
is bounded: if a connection does not close within three seconds the server
closes it and the process exits.

A `ReadHeaderTimeout` of ten seconds, a generous `WriteTimeout` and an
`IdleTimeout` are set, so a stalled client cannot hold a connection open
indefinitely.

## Reach

`--addr` defaults to `127.0.0.1:8080`, so a local `serve` is reachable only from
the machine it runs on. The wiki has no authentication and `--repo` can mount a
repository at `/repo/`, so binding every interface is a decision rather than a
default. With `--mcp` the same bind also publishes the MCP endpoint, which has
no authentication of its own. Set `--addr 0.0.0.0:8080` when you mean to publish
it; the server says on startup when the bind is not loopback, and names the MCP
URL when that endpoint is on.

The container sets `OKF_WIKI_ADDR=0.0.0.0:8080`, because a container that only
listened on its own loopback would be unreachable from the host or from outside
a pod network. That is the intended arrangement for the image, and it is why the
default and the image differ.
