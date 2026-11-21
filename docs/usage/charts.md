---
type: Guide
title: Charts
description: Writing a Vega-Lite chart in a page, with inline data or a data file from the bundle, and what the reader gets online, offline and in print.
tags: [usage, charts, authoring]
---

# Charts

A chart is a fenced block with the language `vega-lite` holding a
[Vega-Lite](https://vega.github.io/vega-lite/) spec as JSON. The spec is the
standard one, so an example from the Vega-Lite gallery or the online editor
works as written, and the source stays readable on GitHub and in any other
Markdown viewer.

````markdown
```vega-lite
{
  "description": "Pages rendered per quarter.",
  "data": {"values": [
    {"quarter": "Q1", "pages": 230},
    {"quarter": "Q2", "pages": 308}
  ]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "quarter", "type": "nominal"},
    "y": {"field": "pages", "type": "quantitative"}
  }
}
```
````

The [Charts test bed](../testbed/charts.md) has a line chart, grouped bars, a
filterable scatter and a quadrant to copy from.

## Data

Data can sit in the spec as `values`, or in a file in the bundle named by `url`
relative to the page:

```json
{"data": {"url": "data/latency.csv"}}
```

The render reads the file and carries its rows in the page, so the chart draws
the same served, under a subpath on a static host, and from an offline folder,
where a browser will not fetch a sibling file. CSV and TSV are parsed at render
time: a column of numbers becomes numbers, a column of `true` and `false`
becomes booleans, and anything else, dates included, stays text, which a
temporal encoding reads. A JSON array is carried as it is; JSON read through
`format.property`, and formats such as TopoJSON, are left for Vega to read. A `.csv`, `.tsv` or
`.json` file is read by its extension; any other file needs `format.type`. The
render stops with the page and line when a chart cannot be built:

- the spec is not valid JSON
- a data file is missing, or is over 2 MB
- a `url` is remote, absolute, or climbs out of the bundle

A spec that is valid JSON but not a valid chart passes the render, and the
figure shows Vega's message in place of the chart.

## What the reader gets

Each chart becomes a figure. The spec's `description` is its caption and its
`title` names it for a screen reader. The data follows in a Data disclosure as
a table of up to 100 rows, so the numbers are there without JavaScript and
before the chart is drawn. A single view with no `width` fills the column, and
the space for the chart is reserved, so the page does not move when it
appears.

Charts take their colours and font from the theme, follow the light and dark
toggle without being redrawn, and print in the light palette. A spec's own
`config` or encoding colours override the theme for that chart. Tooltips,
selections and legend filters from the spec work as they do in Vega-Lite.

## Loading

The chart runtime, about 280 KB compressed, is only sent to a page that has a
chart, and only fetched when the first chart on the page comes near the
viewport. A page without a chart loads nothing extra. The runtime is part of
`make vendor` and the container image, and runs under a Content Security
Policy without `unsafe-eval`. A served render with `--vendor ""` falls back to
pinned CDN builds, which need `unsafe-eval`; an offline folder never uses the
CDN.
