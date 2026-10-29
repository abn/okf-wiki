---
type: Log Entry
title: "Made the tag menu searchable and the map phone-sized"
description: "The tag menu searches and hides one-off tags, and the wiki map uses the height of a narrow screen instead of letterboxing into it."
tags: [log, update, feature]
kind: Update
status: stable
---

The tag menu now opens with a search field and a **One-off tags** toggle. Tags
are ordered by the number of pages that carry them, selected tags stay visible,
and a tag used by one page no longer fills the list by default. That matters
most in a research wiki where almost every page has its own label.

The wiki map lays out fewer columns on a narrow canvas, reserves room for the
group label, and uses a portrait world when the canvas is taller than it is wide.
The map keeps its shape instead of sitting in a landscape letterbox, and names
that do not fit are left to the list, which names every wiki. The list is the
reliable path on a phone.

The map and the wiki scope had already removed the tree-sized drawing: a scoped
wiki opens in about a third of a second, and the busiest tag-hub toggle in the
private tree measured under a quarter of a second. The remaining link and tag
edges are not batched, because the measurements do not justify the extra client
path.

[Graph view](../../../usage/graph.md) covers the tag menu, the map and the list,
and [Graph catalogue](../../../reference/graph.md) records the tag menu and the
cell edges.
