---
type: Guide
title: Static hosting
description: How a rendered wiki behaves on a static host, and why page URLs keep the .html suffix.
tags: [usage, hosting, urls]
---

# Static hosting

A render is a directory of files. A page is written as `<slug>.html`, a
section's home page as `<section>/index.html`, the tag pages under `tags/`, and
the graph view as `graph.html`. A page link names its file. Online, a wiki's
home is linked as a directory URL that the host resolves to `index.html`, and
in a tree so is each folder; an offline render names `index.html` directly
instead. Paths below are shown without the `--base` prefix.

That is the shape a static file server understands: `okf-wiki serve`, an
offline folder opened from `file://`, nginx, Apache, GitHub Pages. An object
store needs an index document for the directory URLs and correct content
types, as it would for any static site. A host that prefers extensionless URLs
rewrites the request on its side. The output does not change to suit one host,
because stripping the suffix from the links would break the offline folder and
any server that maps a request straight to a file.

## Cloudflare Pages

Pages treats `/page.html` as a non-canonical form of `/page` and answers with a
307 to the extensionless path, and `/section/index.html` with a 307 to
`/section/`. Workers static assets calls the same handling
`auto-trailing-slash`. The redirect targets the same document, the browser
keeps the fragment across the hop, and the page's asset URLs are absolute, so
the wiki works unchanged with one extra request per page load. Pages does not
expose a setting that turns the redirect off.

Workers static assets expose that setting, but `html_handling = "none"` stops
the directory URLs resolving as well, so the base (`/wiki/` by default) and a
tree's folder links would then need their own rule. The redirect costs one
request, and removing it means taking on that mapping.

## GitHub Pages

GitHub Pages serves both `/page.html` and `/page` with a 200, so the rendered
links cost no redirect. The [action](github-action.md) deploys the directory as
it is.

## A host that does not rewrite

A minimal file server answers 404 for a path without the suffix, because no
such file exists. That is expected. Link with the file name the renderer wrote,
and let a host that wants extensionless URLs redirect or rewrite on its side.

## Local preview

`okf-wiki serve` answers `/page.html` directly and serves a section's
`index.html` in place, so a local preview takes no redirect. It does not invent
a file for `/page`, because the render does not write one.

See [Configuration](../reference/configuration.md) for `--base` and the served
endpoints, and [Rendering](../design/rendering.md) for why a section home is an
index file.
