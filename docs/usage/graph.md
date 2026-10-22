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
- **Sections** groups pages by their top-level section in a labelled grid. The
  section is the directory title, or a title set with `--sections`; there is no
  `section` frontmatter field. Tag hubs and missing pages get their own cells.
  The grid is fixed; link edges cross between cells when they are visible at
  the current zoom. Changing it clears the current focus and relayouts.
- **Tag hubs** shows or hides tag nodes and tag edges. Hiding them leaves the
  Tags filter and page tags in place; Show Tags is disabled while the hubs are
  collapsed. Changing it also clears the focus and relayouts.
- **Tags** opens a checkbox list of the bundle's tags. A page has to carry every
  selected tag; selected tag hubs stay visible. The count beside each tag is
  the number of pages that carry it. A tag named in the URL but absent from the
  graph still appears in the list, marked as missing, so it can be removed.
- Without Sections, a dense catalogue opens on a readable patch around a page
  with a typical number of connections. With Sections on, it opens on the
  section grid. **Fit view** draws the catalogue as a map, trimming the
  outermost percentile so one stranded page cannot shrink it.
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
and the search catalogue remains the place for text. Sections and Tag hubs are
view settings; they are not part of the shared URL state.

## List view

At wide widths a **List view** sits beside the drawing. At narrow widths it
follows the drawing instead. It lists the nodes and edges that match the
current Filter, Show, Tags, Sections and focus, with connection counts on each
node, and the link text and heading anchor on link edges. The counts in the
status line stay in step with the drawing.

Select a node in the list to select it in the drawing, open its details and
focus its neighbourhood. The list marks the current node. Node rows are normal
buttons; the node list is one tab stop, and the arrow keys move between rows.
The structure can therefore be read and navigated without the SVG.

A large catalogue shows a first page of rows with a **Show all** button in each
section. The status line gives the full counts. The list follows the URL state;
it does not add another parameter.

## Shareable view state

The graph writes its view state into the URL. Focus, depth, Show, Filter and
Tags share one link, for example
`graph.html?focus=page:usage/graph&depth=1&kind=page&q=graph&tag=usage&tag=graph`.
Defaults are omitted, so a plain `graph.html` stays plain. Repeated `tag`
parameters narrow to pages carrying every named tag.

Back and forward restore the controls, the tag checks and the focus state.
Pan and zoom are not in the URL. Changing Show, Tags or focus adds a history
entry. The toolbar's Show all also adds one; the list's Show all only expands
rows. Typing in Filter replaces the current entry so a long query does not
create one entry per keystroke; back and forward move between the discrete
states around it.

## Local graph and backlinks

Every content page has a **Connections** panel near the foot. It lists the
pages that link to the current one, with the source page, its section, the
heading the link sits under, and the text of the link. A page with no incoming
links says so, which is useful for finding orphans. The panel is static HTML;
it works with JavaScript off.

The same panel has a **Local graph** link. It opens the graph focused on the
current page and its direct neighbours. The URL names the node and depth, for
example `graph.html?focus=page:usage/graph&depth=1`. The focus node is selected
and its neighbourhood is lit while the rest of the graph fades. **Show all**
clears the focus, fits the graph and keeps the current filters.

Backlinks are collected within one wiki. Cross-wiki edges are still on the
[graph evolution](../design/graph-evolution.md) list.

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
- [Graph evolution](../design/graph-evolution.md): the roadmap and guardrails.
- [Multi-site trees](multi-site.md#the-graph-catalogue): the aggregate in a
  tree.
