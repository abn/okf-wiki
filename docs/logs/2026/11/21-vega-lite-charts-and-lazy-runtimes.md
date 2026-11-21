---
type: Log Entry
title: "Vega-Lite charts, and diagram and chart runtimes loaded on demand"
description: "A vega-lite fence renders a themed chart with a data table, and the Mermaid and Vega-Lite runtimes are fetched only when a diagram or chart nears the viewport."
tags: [log, update, feature]
kind: Update
status: stable
---

Added charts. A `vega-lite` fence holding a Vega-Lite spec renders as a figure
with a caption and the data as a table, and the browser draws it with Vega-Lite.
A data file named by `url` is read from the bundle and carried in the page, so a
chart draws served, on a static host and from an offline folder. A spec that is
not JSON, a missing data file, or a `url` that leaves the bundle stops the render
with the page and line. Charts take their colours from new `--chart-*` tokens as
`var()` references, so they follow the theme toggle and print in the light
palette without being redrawn.

The runtimes now load on demand. `charts.js` is only in a page with a chart, and
fetches the Vega-Lite bundle when the first chart nears the viewport.
`diagrams.js` fetches Mermaid when the first diagram nears the viewport and
renders each diagram as it arrives, where it used to render every diagram when
the page loaded. An offline page injects the classic Mermaid bundle on demand
instead of loading it with the page. Each runtime is copied into the output only
when some page uses it. `make vendor`, the container image and the release
archives build and carry `vega-bundle.min.js` beside the Mermaid bundles.

[Charts](../../../usage/charts.md) and the
[Charts test bed](../../../testbed/charts.md) are new.
[Rendering](../../../design/rendering.md),
[Branding](../../../design/branding.md),
[Themes](../../../reference/themes.md),
[Configuration](../../../reference/configuration.md),
[Command line](../../../usage/cli.md),
[Offline folder](../../../usage/offline.md),
[Container](../../../usage/container.md) and the
[contributor guide](../../../contribution/index.md) describe the change.
