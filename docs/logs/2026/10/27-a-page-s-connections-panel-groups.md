---
type: Log Entry
title: "A page's Connections panel groups links by source page"
description: "A page's Connections panel groups links by source page."
tags: [log, update, feature]
kind: Update
status: stable
---

A page's Connections panel groups links by source page. A page that links to the current one several times appears once. When it links from more than one place it carries how many links it has, and how many places when the two differ, with one nested row per place that opens the source there. Links under one heading, or above the first heading, share a row. Pages follow the sidebar's section order, links follow document order, and a page that links from a single place keeps the compact one-line row. The graph's selection panel keeps one button per neighbouring page and now says how many catalogue link edges that page contributes, so its totals and its button list no longer disagree. The catalogue records one link edge per anchor, so two links under one heading count once there while the panel counts every distinct link. `usage/graph.md` and `design/rendering.md` record it.
