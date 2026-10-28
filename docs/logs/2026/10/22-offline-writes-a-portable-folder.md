---
type: Log Entry
title: "--offline writes a portable folder that opens from file://"
description: "--offline writes a portable folder that opens from file://."
tags: [log, update, feature]
kind: Update
status: stable
---

`--offline` writes a portable folder that opens from `file://`. Every page gets a relative `<base>`, home links name `index.html`, fragment links name their page, the root redirect points at `index.html`, search data loads from classic scripts, the graph is embedded, and diagram pages load a classic Mermaid bundle instead of calling the CDN. `--repo` is refused. `usage/offline.md` is new, and `reference/configuration.md`, `reference/themes.md`, `design/branding.md`, `design/rendering.md`, `usage/cli.md`, `usage/graph.md` and `usage/multi-site.md` say so.
