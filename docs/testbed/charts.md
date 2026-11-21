---
type: Guide
title: Charts
description: Vega-Lite charts in the shapes a docs bundle uses, drawn from inline data and from data files beside the page.
tags: [testbed, charts]
---

# Charts

A `vega-lite` fence holds a Vega-Lite spec as JSON. The render turns it into a
figure with the spec, a caption from its `description`, and the data as a table,
and the browser draws the chart when it comes near the viewport. The chart
runtime is only fetched by a page that has a chart, and only once a chart is
about to be seen, so the charts further down this page cost nothing until you
scroll to them. See [Charts](../usage/charts.md) for how to write one.

## Line chart from a data file

The data is `data/latency.csv`, named relative to this
page. The render reads it and carries it in the page, so the chart draws from a
static host and from an offline folder alike.

```vega-lite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v6.json",
  "description": "Median render, search and serve time per week, in milliseconds.",
  "data": {"url": "data/latency.csv"},
  "height": 260,
  "mark": {"type": "line", "strokeWidth": 2},
  "encoding": {
    "x": {"field": "week", "type": "quantitative", "title": "Week"},
    "y": {"field": "ms", "type": "quantitative", "title": "Median ms"},
    "color": {"field": "service", "type": "nominal", "title": null}
  }
}
```

## Grouped bars from inline data

```vega-lite
{
  "description": "Pages rendered per quarter, by service.",
  "data": {
    "values": [
      {"quarter": "Q1", "service": "render", "pages": 230},
      {"quarter": "Q1", "service": "search", "pages": 330},
      {"quarter": "Q1", "service": "serve", "pages": 395},
      {"quarter": "Q2", "service": "render", "pages": 308},
      {"quarter": "Q2", "service": "search", "pages": 332},
      {"quarter": "Q2", "service": "serve", "pages": 398},
      {"quarter": "Q3", "service": "render", "pages": 426},
      {"quarter": "Q3", "service": "search", "pages": 431},
      {"quarter": "Q3", "service": "serve", "pages": 452},
      {"quarter": "Q4", "service": "render", "pages": 455},
      {"quarter": "Q4", "service": "search", "pages": 512},
      {"quarter": "Q4", "service": "serve", "pages": 509}
    ]
  },
  "height": 220,
  "mark": {"type": "bar", "cornerRadiusTopLeft": 3, "cornerRadiusTopRight": 3},
  "encoding": {
    "x": {"field": "quarter", "type": "nominal", "title": null},
    "xOffset": {"field": "service"},
    "y": {"field": "pages", "type": "quantitative", "title": "Pages"},
    "color": {"field": "service", "type": "nominal", "title": null},
    "tooltip": [
      {"field": "service"},
      {"field": "quarter"},
      {"field": "pages", "type": "quantitative"}
    ]
  }
}
```

## Scatter with a legend that filters

Click a legend entry to pick out one kind of page; shift-click adds another.
Hover a point for its values.

```vega-lite
{
  "description": "Words against outbound links for each page, by page kind.",
  "data": {"url": "data/pages.json"},
  "height": 260,
  "params": [{
    "name": "kind",
    "select": {"type": "point", "fields": ["kind"]},
    "bind": "legend"
  }],
  "mark": {"type": "circle", "size": 48},
  "encoding": {
    "x": {"field": "words", "type": "quantitative", "title": "Words"},
    "y": {"field": "links", "type": "quantitative", "title": "Links"},
    "color": {"field": "kind", "type": "nominal", "title": null},
    "opacity": {"condition": {"param": "kind", "value": 0.9}, "value": 0.12},
    "tooltip": [
      {"field": "kind"},
      {"field": "words", "type": "quantitative"},
      {"field": "links", "type": "quantitative"}
    ]
  }
}
```

## Quadrant

A layered spec: the points, two rules at the axis midpoints, and a label in
each corner. This is the shape of an effort and impact grid.

```vega-lite
{
  "title": "Effort and impact",
  "description": "Candidate work items placed by effort and impact, split into four quadrants at the midpoint of each axis.",
  "data": {
    "values": [
      {"item": "Lazy diagrams", "effort": 2, "impact": 8},
      {"item": "Chart fence", "effort": 5, "impact": 9},
      {"item": "Theme tokens", "effort": 3, "impact": 6},
      {"item": "PNG export", "effort": 4, "impact": 2},
      {"item": "Keyboard points", "effort": 8, "impact": 6},
      {"item": "Server render", "effort": 9, "impact": 4},
      {"item": "Spec linting", "effort": 6, "impact": 7}
    ]
  },
  "height": 300,
  "layer": [
    {
      "mark": {"type": "rule", "strokeDash": [4, 4]},
      "encoding": {"x": {"datum": 5, "type": "quantitative"}}
    },
    {
      "mark": {"type": "rule", "strokeDash": [4, 4]},
      "encoding": {"y": {"datum": 5, "type": "quantitative"}}
    },
    {
      "data": {"values": [{"x": 0.4, "y": 9.4, "label": "Quick wins"}, {"x": 0.4, "y": 0.6, "label": "Fill-ins"}]},
      "mark": {"type": "text", "align": "left", "baseline": "middle", "fontWeight": 600, "opacity": 0.55},
      "encoding": {
        "x": {"field": "x", "type": "quantitative"},
        "y": {"field": "y", "type": "quantitative"},
        "text": {"field": "label"}
      }
    },
    {
      "data": {"values": [{"x": 9.6, "y": 9.4, "label": "Big bets"}, {"x": 9.6, "y": 0.6, "label": "Money pits"}]},
      "mark": {"type": "text", "align": "right", "baseline": "middle", "fontWeight": 600, "opacity": 0.55},
      "encoding": {
        "x": {"field": "x", "type": "quantitative"},
        "y": {"field": "y", "type": "quantitative"},
        "text": {"field": "label"}
      }
    },
    {
      "mark": {"type": "point", "filled": true, "size": 70},
      "encoding": {
        "x": {"field": "effort", "type": "quantitative", "scale": {"domain": [0, 10]}, "title": "Effort"},
        "y": {"field": "impact", "type": "quantitative", "scale": {"domain": [0, 10]}, "title": "Impact"},
        "tooltip": [{"field": "item"}, {"field": "effort"}, {"field": "impact"}]
      }
    },
    {
      "mark": {"type": "text", "dx": 8, "align": "left", "fontSize": 11},
      "encoding": {
        "x": {"field": "effort", "type": "quantitative"},
        "y": {"field": "impact", "type": "quantitative"},
        "text": {"field": "item"}
      }
    }
  ]
}
```

## A spec Vega rejects

Valid JSON that is not a valid chart passes the render and fails in the
browser. The figure shows Vega's message where the chart would be.

```vega-lite
{
  "description": "A mark type Vega-Lite does not have.",
  "data": {"values": [{"a": 1}]},
  "mark": "pyramid",
  "encoding": {"x": {"field": "a", "type": "quantitative"}}
}
```
