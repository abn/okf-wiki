---
type: Guide
title: Graph view
description: Opening the structural graph in the browser, filtering it, downloading it, and embedding it in the page.
tags: [usage, graph]
---

# Graph view

The graph control in the header opens the structural graph at
`<base>graph.html`. A single bundle writes one page at its base. A multi-site
tree writes one at the tree base, and every wiki's header points there, so the
view spans the tree the way search does. Opening it fetches `graph.json`; no
other page loads the catalogue, and `--graph-inline` removes even that request.

The drawing has one node per page and per tag. A page linked from the bundle
but missing from it appears as a dashed node, so a broken link is visible here
as well as in the file. Edges are solid for Markdown links and dashed for
frontmatter tags.

## Reading it

- **Filter** narrows by title, section, tag, or node ID. Matching nodes stay
  lit and the rest fade.
- **Show** switches between all nodes, pages, tags, and missing pages.
- A dense catalogue opens on a readable patch around a page with a typical
  number of connections, not on the whole cloud. **Fit view** draws the
  catalogue as a map, trimming the outermost percentile so one stranded page
  cannot shrink it.
- **Labels** hides or shows the names. At most 50 names are drawn in one view,
  and one is chosen only where it fits without covering another name or another
  node. Names keep their screen size at every zoom, so zooming in shows more.
  Hover a node or select it to reveal its own name.
- Select a node for its kind, section, tags, and incoming and outgoing counts.
  The rest of the drawing fades to that node's neighbourhood: two hops for a
  page (one when that set is very large), one for a tag. When the neighbourhood
  fits, the view frames it.
- The panel lists the node's connections as buttons, forty at a time with the
  rest behind one more click. Select one to move to that node, or use **Open
  page**, **Open tag page**, or **Clear selection**. Escape also clears the
  focus. With the keyboard, tab to the graph, use the arrow keys
  to move between nodes, and press Enter for the details.
- Drag a node to move it. Scroll to zoom, drag the background to pan, and
  select **Fit view** or double-click the background to fit the graph again.
  An edge whose endpoints are both outside the view is not drawn, and an
  overview zoom hides edges unless a node is selected.
- **Download JSON** saves the same bytes the view loaded as `graph.json`.

The page carries no page text and no embeddings. It is the structural layer,
and the search catalogue remains the place for text.

## Embedding the catalogue

```bash
okf-wiki render --content ./docs --out ./public --graph-inline
```

`--graph-inline` writes the catalogue into the page instead of leaving the
client to fetch it. The page is larger, and opening the view makes no second
request. That helps when a viewer or proxy blocks `fetch` of a sibling file.
The rest of the site still resolves through `--base`, so a local preview is
served rather than opened from disk; `okf-wiki serve` is the short path. An
offline render (`--offline`) inlines the catalogue automatically.

The environment equivalent is `OKF_WIKI_GRAPH_INLINE=true`.

## The reserved path

The view needs `graph.html` at the render root. A Markdown page `graph.md` at
that root would render to the same path, and so would a static file called
`graph.html`, so either one fails the render with the path named rather than
being overwritten. Rename the page or the file.

In a multi-site tree only the root is reserved. A `graph.md` inside a wiki
below the root is an ordinary page of that wiki, because the tree's view is at
the tree base.

## See also

- [Graph catalogue](../reference/graph.md): the schema, node and edge fields,
  and how the file is built.
- [Multi-site trees](multi-site.md#the-graph-catalogue): the aggregate in a
  tree.
