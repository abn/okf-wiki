---
type: Reference
title: Graph catalogue
description: The structural graph.json written beside the search catalogue, the browser view that draws it, and the schema behind both.
tags: [reference, graph, schema]
---

# Graph catalogue

Every render writes `graph.json` beside the search catalogue. It is the
structural graph of the bundle: one node per page and per tag, one edge per
in-bundle Markdown link and per frontmatter tag. In a tree, a relative Markdown
link that climbs to a sibling wiki adds an edge to the aggregate only. It is
derived from the files alone, so two renders of an unchanged bundle produce the
same bytes.

## Where it is written

A single bundle writes `graph.json` under its base, next to
`search-index.json`. In a multi-site tree every wiki keeps its own file at its
own path, and the tree writes one aggregate at its base after the wikis have
rendered. The aggregate prefixes each wiki's node IDs with its path under the
tree root, so two wikis can both have an `index` page without colliding. A
relative Markdown link that climbs out of one wiki becomes an edge between the
two namespaced IDs in the aggregate; the per-wiki catalogue keeps only its own
edges. A wiki that could not be read contributes nothing, and a tree with no
readable wiki still gets an empty catalogue rather than a missing file.

## Schema

`version` is `1`. A reader checks it before relying on a field.

### Nodes

| Field | Meaning |
| :--- | :--- |
| `id` | `page:` plus the page slug, or `tag:` plus the tag slug. An aggregate puts `<siteKey>:` in front of it. |
| `title` | the page title, or `#` plus the tag's first spelling |
| `label` | a shortened title for the view, present only when it differs from `title`; a reader that does not know the field falls back to `title` |
| `url` | the rendered page, absent on a stub |
| `kind` | `page` or `tag` |
| `section` | the section title, on pages |
| `sectionId` | the section directory ID, when present |
| `type`, `status` | frontmatter values, when present |
| `tags` | the page's frontmatter tags |
| `stub` | true for a page an in-bundle link names and the bundle does not contain |
| `site`, `siteKey` | the wiki's title and path, on every node in an aggregate |

Both kinds carry a prefix because they share one ID space: a file may be named
`tag:foo.md`, and its page must not swallow the hub for a tag called `foo`.

A stub has no `url`, because there is no page to open. It exists so a broken
link is visible in the catalogue instead of disappearing from it.

### Edges

| Field | Meaning |
| :--- | :--- |
| `from`, `to` | node IDs |
| `kind` | `link`, `tag`, `previous` or `next` |
| `anchor` | the heading anchor of the section the link sits in |
| `text` | the link text |

`anchor` and `text` belong to link edges. A link repeated under one heading
produces one edge, and a link from a page to itself is left out. Previous and
next are structural page-order edges inside one section; they carry no anchor or
text.

A link edge comes from an in-bundle Markdown link. In an aggregate, a relative
link that climbs out of one wiki and lands on a page in another wiki also
becomes a link edge; the two ends carry namespaced IDs. A link that leaves the
tree, points at an asset, or is external is not one, because the catalogue
follows what the renderer can serve: an escaping link is a `/repo/` target at
most, and an asset has no page node to point at. Page order adds the
`previous` and `next` edges separately.

## Example

```json
{
  "version": 1,
  "nodes": [
    {
      "id": "page:guide/install",
      "title": "Install",
      "url": "/wiki/guide/install.html",
      "kind": "page",
      "section": "Guide",
      "sectionId": "guide",
      "tags": ["setup"]
    },
    {
      "id": "tag:setup",
      "title": "#setup",
      "url": "/wiki/tags/setup.html",
      "kind": "tag"
    }
  ],
  "edges": [
    {
      "from": "page:guide/install",
      "to": "tag:setup",
      "kind": "tag"
    }
  ]
}
```

## What it does not carry

Page text, embeddings and source records. The catalogue is the structural layer,
kept deterministic and small. The search catalogue already carries the text, and
a semantic layer would be a separate artifact with its own provenance rather
than a second meaning for this one.

## The browser view

Every render writes `graph.html` at the render root: the bundle base for a
single bundle, and the tree base for a multi-site tree. The header control on
every page links to it. In a tree every wiki points at the one tree page, which
draws the aggregate; the per-wiki files remain beside the wikis for an exporter.
A root that is itself a wiki writes the page from its own render.

The page fetches `graph.json` when it opens, so the catalogue is downloaded on
demand rather than with every page. `--graph-inline` embeds the same bytes in
the page and the client reads them instead, which removes the second request.
An offline render embeds it automatically.

In a tree with no `site`, the view derives a map from the aggregate: one node
per wiki, its radius following the page count, grouped by the wiki's top-level
directory rows. A cross-wiki link becomes one edge between the two wiki nodes,
weighted by how many links cross. The map is a view of the same catalogue, not a
new node in it; `graph.json` still holds pages and tags only. Selecting a wiki
sets `site` and draws that wiki's pages.

The page carries its view state in the query string. Defaults are omitted.

| Parameter | Meaning |
| --- | --- |
| `focus` | node ID to select and focus, for example `page:usage/graph` |
| `depth` | focus distance in hops, clamped to 1 through 3 when `focus` is present |
| `kind` | `all`, `page`, `tag` or `stub`; absent means all nodes |
| `q` | text matched against title, section, type, tag, ID and site |
| `tag` | tag slug, repeated for more tags; a page must carry every named tag |
| `site` | wiki key to scope an aggregate to one wiki, for example `site=projects%2Froundhouse`; absent opens the wiki map |
| `type` | `all`, `content`, or a type slug such as `log-entry`; `content` hides log entries |

Repeated tags combine with AND. A page's Connections panel uses `focus` and
`depth` to open the graph around that page, for example
`graph.html?focus=page:usage/graph&depth=1`. An unknown focus shows a message
rather than failing. An unknown tag stays in the tag list, marked as missing,
so a reader can remove it. The client writes these parameters on change and
restores them on back and forward. Pan and zoom are not stored. The page also
builds a list of the visible nodes and edges beside the drawing, from the same
catalogue, for keyboard and screen-reader use. The drawing can group pages by
section and collapse the tag hubs; those two are view settings rather than URL
parameters. The force layout is seeded from the visible catalogue, so positions
repeat on every visit; they are not stored in the file. Labels are tiered:
tag hubs and the busiest quarter of the connected pages name the overview, and
ordinary page names join as the reader zooms in.

A wiki's header control opens the aggregate with `site=<siteKey>`, so the view
starts in that wiki. The tree index, a group index and a single bundle keep the
unscoped link. The Wiki control offers every wiki in the tree, grouped by its
top-level directory, and All wikis clears the scope. In a tree that carries log
entries, the Type control opens on `content`, so the first view is the pages
that carry navigation; `all` and the named types bring the rest back. The status
line names the scope and reports how many log entries are hidden. A catalogue
with more than 500 nodes opens with tag hubs collapsed; the checkbox and the tag
filter still work. At a section-clustered overview the drawing shows one weighted
edge per pair of cells; the individual links join when the reader zooms in or
selects a node.
See [Graph view](../usage/graph.md) for the controls and the reserved path.

The view is a client of this schema, not a second one. It reads `version`,
`nodes`, `edges`, `kind`, `stub`, `url`, `title`, `label`, `section`,
`sectionId`, `type`, `status`, `tags`, `site` and `siteKey`;
a field added here does not change the drawing unless the client is taught to
use it.

## Fetching it

The file is served like any other output, so the server's gzip and ETag handling
applies. The graph page fetches it when opened, and an exporter reads it
directly:

```bash
curl -s http://127.0.0.1:8080/wiki/graph.json | jq '.nodes | length'
```

An exporter reads the same file to load the bundle into a graph store. Node IDs
are stable node identities, and `url` is the address to open for a page. Edge
properties name the section and link text, which is what makes the structure
reusable outside the wiki.
