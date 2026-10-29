---
type: Design
title: Graph evolution
description: The working list for the structural graph, its guardrails, and the separate semantic layer that comes later.
tags: [design, graph, roadmap]
---

# Graph evolution

The graph is a structural catalogue today: pages and tags as nodes, Markdown
links and frontmatter tags as edges. This is the working list for what comes
next. It is deliberately ordered: navigation and scale first, then schema, then
the semantic layer.

## Guardrails

- `graph.json` stays deterministic, additive and free of page text, vectors and
  model output. A reader can rebuild it from the bundle alone.
- Semantic knowledge is a separate artifact, not a new meaning for `graph.json`.
  It carries `layer`, model, prompt version, generation date, source hash,
  confidence and evidence.
- The catalogue loads on demand, like the search index. A new view feature does
  not make every page download it.
- No new client dependency without a page-level contract that can be replaced.
  The current SVG view is intentionally small.
- Node IDs stay stable. A multi-site aggregate namespaces them by wiki, and a
  cross-wiki edge uses the namespaced IDs.
- Schema changes are additive in v1. A breaking shape change needs a new
  version and a reader that checks it.
- Every view feature works offline and has a keyboard and non-visual path. The
  graph is navigation, not decoration.
- The structural graph stays the default. The semantic layer is opt-in and
  never silently mixed with structural edges.

## To do

### Structural navigation

- [x] Local graph and backlinks: a page's Connections panel and a focused
  neighbourhood in the graph view.
- [x] Shareable graph state: focus, depth, kind, text and tags in the URL, with
  working back and forward.
- [x] Accessible list view: a table or list of nodes, edges and connections
  beside the drawing.
- [x] Search integration: open a search hit in the graph, and highlight graph
  matches from the search modal.
- [x] Scope by wiki and by page type: the aggregate opens on a wiki's content
  pages, the URL carries `site` and `type`, and the list groups by wiki.

### Scale and readability

- [x] Collapse tag hubs and cluster by section.
- [x] Stable or precomputed layout, so a graph looks the same on every visit.
- [x] Semantic zoom: labels for hubs first, page names as the reader zooms in.
- [x] Path finding between two pages.
- [x] Graph health: orphans, broken links, tag-only pages, cycles and the most
  connected pages.

### Schema and multi-site

- [x] Cross-wiki edges, using the namespaced aggregate IDs.
- [x] Typed structural edges beyond `link` and `tag`: previous and next page
  order edges.
- [ ] Export formats such as GraphML, DOT, CSV and JSON-LD.
- [ ] Graph diff between two renders or commits.

### Semantic layer

- [ ] A separate `semantic.json` artifact with provenance and a versioned
  reader.
- [ ] Concept nodes and typed relations such as prerequisite, related,
  contradicts and part of.
- [ ] Confidence filtering and an evidence panel.
- [ ] Optional vectors in a sidecar, never in the structural catalogue.

## Current

Local graph and backlinks, shareable view state, the accessible list view,
collapsed tag hubs, section clusters, a deterministic layout, semantic zoom,
cross-wiki edges, typed page-order edges, and wiki and type scope are shipped.
The next item is a first screen that maps the wikis themselves, then graph diff
between two renders or commits.
