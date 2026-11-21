---
type: Log Entry
title: "Labels for header icons, and a width icon that says width"
description: "Every icon-only header control shows one styled tooltip on hover or keyboard focus, and the width toggle's icon gains edge bars."
tags: [log, update, feature]
kind: Update
status: stable
---

The header's icon-only controls had no visible labels: the graph, download and
width buttons relied on slow, unstyled `title` tooltips, and the theme toggle had
none. Each now shows the same tooltip, a near-black pill in light and near-white
in dark, 6px under the button. It appears after a 300ms hover, or at once on
keyboard focus, and not at all on touch, where the accessible name still names
the control. The label at the end of the row is right-aligned so it stays on
screen. The labels are Knowledge graph, Download bundle (.zip), Dark theme or
Light theme, and Full width or Normal width, and the two toggles keep their
label and accessible name in step with their state. The `title` attributes are
gone, so two tooltips never stack.

The width icon gains bars at its edges, arrows out to widen and arrows in to
narrow, so it reads as page width rather than as a swap.
[Themes](../../../reference/themes.md) documents the `data-tip` hook.
