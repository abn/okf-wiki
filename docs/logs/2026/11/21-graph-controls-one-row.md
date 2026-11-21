---
type: Log Entry
title: "Graph controls take one row, the map comes first"
description: "The graph view's controls fit one row on a desktop and collapse behind a Filters button on a phone, with Fit moved onto the drawing."
tags: [log, update, feature]
kind: Update
status: stable
---

Reworked the controls on the graph view. On a desktop they took three rows,
and on a phone they filled the first screen and pushed the map below the fold.
They now fit one row: Filter, Wiki, Type, Show and Tags, a Display menu for
Sections, Tag hubs and Labels, Path between pages, and an Export menu holding
Download JSON and Open raw JSON. Download JSON is no longer a filled button,
since nothing on the page is a primary action. Fit sits on the drawing, top
right, and the Type and Show labels are read by screen readers but not shown.

At 720px and narrower the row is Filter and a Filters button that opens the
rest as a two-column panel, and the status line moves under the map. The hint
follows the input device: pinch on a touch screen, scroll elsewhere.
[Graph view](../../../usage/graph.md) describes the layout, and
[Themes](../../../reference/themes.md) lists the new ids.
