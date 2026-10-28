---
type: Log Entry
title: "Sibling wiki links become cross-wiki edges"
description: "A relative Markdown link that climbs to a sibling wiki becomes a cross-wiki edge in the tree's aggregate graph."
tags: [log, update, feature]
kind: Update
status: stable
---

A relative Markdown link that climbs to a sibling wiki becomes a cross-wiki edge in the tree's aggregate graph. Both ends carry namespaced IDs, the per-wiki catalogue and Connections panel stay inside their wiki, and the client draws the edge in the accent colour and labels it `cross-wiki` in the list. `usage/multi-site.md`, `usage/graph.md`, `reference/graph.md`, `design/rendering.md` and `design/graph-evolution.md` record it.
