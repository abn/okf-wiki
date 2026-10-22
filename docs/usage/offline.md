---
type: Guide
title: Offline folder
description: Rendering a wiki that opens from file://, with search, diagrams and the graph working without a server.
tags: [usage, offline]
---

# Offline folder

`--offline` writes a folder that opens directly from disk. Pages use relative
URLs, search loads local data, diagrams use a local runtime, and the graph is
embedded. No request leaves the folder.

```bash
okf-wiki render --content ./docs --out ./site --offline
```

Open `site/wiki/index.html`, or `site/index.html`, which redirects to it. The
environment equivalent is `OKF_WIKI_OFFLINE=true`.

## What changes

- Each page gets a `<base>` pointing at the render root, and every asset and page
  link is relative to it. The folder can be moved without breaking links.
- Home links name `index.html`. A `file://` directory URL opens a listing, not
  the wiki's home page.
- Same-page fragments name their page explicitly, because a `<base>` otherwise
  sends `#section` to the root. This covers the table of contents, skip link,
  footnotes, tag anchors and heading permalinks.
- Search index, text and tag catalogues are written as classic scripts beside
  their JSON files and loaded lazily. This is what makes search work when
  `fetch` of a sibling file is blocked.
- The graph catalogue is embedded, so the graph view needs no request.
- Pages with a diagram load `vendor/mermaid-bundle.min.js`, a classic build of
  Mermaid and ELK. Pages without a diagram do not load it. There is no CDN
  fallback from `file://`; if the runtime is missing, the render fails rather
  than pretending to be offline.
- The root redirect names the wiki's `index.html` rather than a directory.

## Moving it

Keep the folder together. Links are relative to each page's render root, so a
copied or renamed folder still works. `--base` still names the directory inside
`--out`; it is not a URL prefix in this mode.

## Limits

`--offline` refuses `--repo`, because a repository mount is a server endpoint
and has no file to open. A custom theme can still write an in-document link in
its own inline script; it must build that link from `{{.PageHref}}`. Links in
theme HTML are rewritten like page links.

Search and diagrams are local, but the browser still applies its normal
`file://` restrictions. This mode does not add a service worker, and it does not
make a page work when a browser profile blocks local scripts.

## Browser support

The folder is tested from `file://` in current desktop Chrome and Firefox. Both
load the classic scripts and the classic Mermaid bundle from disk; the mode does
not depend on `fetch` or module imports. Safari has not been tested.

## See also

- [Command line](cli.md): the flags on the render command.
- [Graph view](graph.md): the graph control and its inline catalogue.
- [Multi-site trees](multi-site.md): the same mode over a tree.
