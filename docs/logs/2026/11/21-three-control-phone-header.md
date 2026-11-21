---
type: Log Entry
title: "A three-control phone header"
description: "The width toggle hides at 860px and below, and the bundle download moves into the drawer at 620px and below, leaving graph, search and theme in a phone header."
tags: [log, update, fix]
kind: Update
status: stable
---

A phone header showed five icon buttons, one of them a width toggle that does
nothing on a one-column layout. The width toggle now hides at 860px and below,
where the layout is already one column. At 620px and below the bundle download
moves into the drawer as the labelled row that used to appear only under 350px,
and the controls sit 4px apart. A phone header is then the brand, graph, search
and theme, each a 44px target. The graph view has no drawer, so it keeps the
download in the header and hides its own graph control, as it did before at the
narrowest widths.

The narrower header rules at 430, 389, 360 and 349px are gone, since nothing is
left for them to do. [Branding](../../../design/branding.md),
[Rendering](../../../design/rendering.md) and
[Themes](../../../reference/themes.md) describe the phone header.
