---
type: Log Entry
title: "Scoped the graph to a wiki and content type"
description: "The graph opens on a wiki's content pages, groups the list by wiki, and leaves log entries and tag hubs out of the first view."
tags: [log, update, feature]
kind: Update
status: stable
---

The graph view now treats the wiki and the page type as scopes rather than
filters. A wiki's header control opens the tree aggregate with
`?site=<siteKey>`, the toolbar's Wiki control changes that scope, and All wikis
clears it. A scope change rebuilds the visible graph, so the force layout and
the list only carry the selected pages instead of dimming a tree-sized drawing.

The Type control opens on content pages when a tree carries log entries. Log
entries are chronological, they account for a large share of a private tree, and
most of them have no link edges, so they no longer decide the first screen. The
list groups nodes by wiki and section, names each wiki with its page count, and
spreads its first page across every wiki. A catalogue above 500 nodes opens
with tag hubs collapsed.

[Graph catalogue](../../../reference/graph.md) records the new `site` and `type`
parameters, [Graph view](../../../usage/graph.md) documents the controls and the
grouped list, and [Multi-site trees](../../../usage/multi-site.md) records the
scoped header link.
