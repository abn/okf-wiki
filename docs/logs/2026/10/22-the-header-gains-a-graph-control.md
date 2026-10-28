---
type: Log Entry
title: "The header gains a graph control"
description: "The header gains a graph control."
tags: [log, update, feature]
kind: Update
status: stable
---

The header gains a graph control. It opens `graph.html` at the render root, which draws `graph.json` on demand: pages and tags as nodes, links and tags as edges, with filter, kind, label, detail and download controls. A multi-site tree writes one view at its base and every wiki links to it, so it draws the aggregate that the search index already mirrors. `--graph-inline` embeds the catalogue in the page and removes the second request. The view reserves `graph.html` at the root and fails the render with the path named if a page or static file already owns it. `usage/graph.md` is new, and `reference/graph.md`, `reference/themes.md`, `reference/configuration.md`, `design/rendering.md`, `usage/cli.md` and `usage/multi-site.md` say so.
