---
type: Log Entry
title: "Mapped the wikis before drawing their pages"
description: "A tree opens on a wiki map sized by page count, and a scoped wiki gives each section room in proportion to its pages."
tags: [log, update, feature]
kind: Update
status: stable
---

The tree's graph has a first screen above the page view now. All wikis opens a
map with one node per wiki, sized by page count and grouped by top-level
directory. A cross-wiki link is one weighted edge between the two wikis, so a
tree with no cross-wiki links reads as the directory it is instead of a cloud of
thousands of dots. Selecting a wiki opens its pages, and the Wiki control
returns to the map.

Inside a wiki the section grid uses each section's page count. A log archive
gets the room its pages need and a two-page section does not. At overview scale
the drawing connects section cells, weighted by the links across them; the
individual links appear when the reader zooms in or selects a node. The graph
catalogue is unchanged: the map and the cell edges are derived by the client
from the same `graph.json`.

[Graph view](../../../usage/graph.md) covers the map, the proportional grid and
the cell edges, [Graph catalogue](../../../reference/graph.md) records the
derived map, and [Multi-site trees](../../../usage/multi-site.md) records where
the map appears in a tree. [Graph evolution](../../../design/graph-evolution.md)
moves the map from the roadmap to the shipped list.
