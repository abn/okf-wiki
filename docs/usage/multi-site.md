---
type: Guide
title: Multi-site trees
description: Publishing a directory of wikis as one site, with an index at every group.
tags: [usage, multi-site]
---

# Multi-site trees

A bundle is one wiki. `--multi-site` turns the content directory into a tree of
them, so a collection of projects publishes as one site with an index at every
level, rather than as separate renders stitched together afterwards.

## What a wiki is

A directory holding an `index.md` is a wiki. Every other directory is a group: a
folder that exists to hold wikis and other groups. That distinction is the whole
rule, and it is why a wiki's own sub-directories stay its sections. A directory
is never both.

A directory symlink counts as a directory, so a tree is assembled by linking
bundles in from wherever their checkouts live rather than copying them, and the
content directory itself may be a symlink. A directory already walked is not
walked again, so a link cycle stops rather than expanding. A linked bundle's
non-Markdown files travel with it: a wiki linked in from its own checkout still
serves its images and downloads. The link is resolved where the render runs, so a
render inside a container needs the link targets reachable there as well, mounted
at the same absolute paths.

```text
content/
  clients/           a group, named by its .meta.json
    example/         a group
      operations/    a wiki: an index.md at its root
      strategy/      a wiki
  homelab/           a wiki at the top level
```

## Rendering and serving

```bash
okf-wiki render --content ./content --out ./_site --multi-site --base /
okf-wiki serve  --content ./content --out .scratch/wiki --multi-site --base /
```

The output mirrors the tree. Each wiki is written at its path under the base,
with its own assets and links, and each group is given an `index.html` listing
the wikis and groups directly inside it. The wikis lead as cards; the folders
follow under a "Folders" heading as compact rows, because a folder is a
container on the way to a wiki rather than a destination in itself:

```text
_site/
  index.html                the root group's index
  clients/
    index.html              the clients group
    example/
      index.html            the example group
      operations/           the wiki, unchanged
        index.html
        wiki.css
        ...
      strategy/             the wiki
  homelab/
    index.html              the wiki
```

`--base` prefixes the whole tree, so `--base /family/` publishes the same shapes
under `/family/` and the output root redirects there. A `serve` serves the same
layout from one port, and the group indexes are the pages that follow the tree.
A group has no assets of its own: its pages use the theme and the search
catalogue written at the base, so with `--base /family/` those files sit under
`_site/family/` rather than at the output root, which the two paths share only
when the base is `/`.

With `--bundle`, each wiki also writes its own `okf-bundle.zip` at its base and
offers it from its pages, holding that wiki's content root: its own files, or
the whole tree when the root is itself a wiki. A group index has no bundle of
its own, so it carries no control; the wikis inside it each offer theirs.

## Naming a group

An entry on a group index is named from, in order:

- a `.meta.json` in the directory, `{"title": ..., "description": ...}`
- the frontmatter of the entry's own `index.md`, when the entry is a wiki
- a `README.md`, its first heading and its first line of prose
- the directory name, title-cased

None is required. A folder still lists, named after its directory, so a tree
needs no metadata to work.

## Brand and breadcrumb

One brand stands for the whole tree. The brand mark and the wordmark link to the
tree index, not to the wiki the reader is in, so they mean "the deployment home"
and work from any depth. There is no per-wiki brand and no separate Home control:
the brand is the home, and the breadcrumb says where the reader is.

The breadcrumb becomes a trail from the tree root down to the current wiki:

```text
Wiki / Clients / Example / Operations / Guides / Setup
```

The tree title (`--title`, e.g. "Wiki") links to the tree index, each folder
links to that group's index, the wiki title links to the wiki's own home, and the
section and page crumbs follow as they do in a single bundle. The current page is
always the last crumb. On a group index the trail ends at the group's name, and
at the tree root it is the tree title alone, never a link to itself. A tag page
keeps the trail of the wiki it belongs to and then reads `Tags / #tag`, so the
reader who followed a tag chip from a page keeps the same trail behind them. A
single bundle is unchanged: still `Docs / section / page`, and still
`Docs / Tags / #tag` on a tag page, with the brand on its own base.

## Watching

`serve --multi-site --watch` re-renders the whole tree when anything under it
changes: an edit to a page, a page added, or a new wiki directory. The change is
live on the next page load. See [Live reload](cli.md#live-reload).

## The search index

Search spans the whole tree. Each wiki keeps its own `search-index.json` and
the text companion `search-content.json`, so it remains a site of its own, and
after every wiki has rendered the tree writes one aggregate pair at its base.
The aggregate merges the wikis' page tables into one and rebases each entry onto
it, so a page's url and titles are still stored once in the tree-wide file, and
its content file concatenates the wikis' entry texts in the aggregate's entry
order. Every wiki and group fetches that aggregate, so a term that appears in a
sibling wiki is found from here and answers with a heading naming the wiki it
came from. A wiki that could not be read contributes nothing to it. A content
root that is itself a wiki is a tree of one: its own index is the one at the
base, with no aggregate written over it.

The modal can be narrowed to the current wiki with the All wikis / This wiki
toggle in its footer. The toggle appears only in a tree, where the aggregate
lives at the tree base rather than the wiki's own base; on a single wiki the two
are the same and it is hidden.

A group index has no pages, so a search run from one fetches the tree aggregate
and reaches every wiki in the tree, a sibling branch as much as the wikis below
the group. The tag catalogue stays per wiki, and `#tag` searches the current
wiki's tags, because a tag page is a page of the wiki that carries the tag.

## The graph catalogue

Each wiki writes its own `graph.json` beside its search catalogue, and after
every wiki has rendered the tree writes one aggregate at its base. The aggregate
prefixes every node ID with the wiki's path under the tree root, so two wikis
can both own an `index` page, and a node's URL already points into the wiki it
came from. A relative Markdown link that climbs to a sibling wiki becomes an
edge between the two namespaced IDs; the wiki's own catalogue keeps only its
own edges. A wiki that could not be read contributes nothing. A tree whose root
is itself a wiki keeps that wiki's own catalogue at the base, with no aggregate
written over it.

The tree also writes one `graph.html` at its base. Every wiki and group links to
it from the header, and it draws the aggregate. Opened from the tree index or a
group index, a tree of more than one wiki shows a map of the wikis, sized by
page count and grouped by top-level directory; a tree of one wiki opens its
pages. Opened from a wiki it carries `?site=<siteKey>` and shows that wiki's
pages. A cross-wiki link is an edge between the two wikis on the map. The Wiki
control changes the scope, and the list groups by directory or section. The
per-wiki files stay where they are for an exporter.
`--graph-inline` embeds the aggregate once in that page rather than a copy in
every wiki. A root that is itself a wiki writes the page from its own render,
with its own catalogue. See [Graph view](graph.md). An offline render
(`--offline`) keeps the same tree shape: relative URLs per wiki, search data as
scripts, one embedded tree graph, and a classic Mermaid bundle where a wiki has
diagrams.

## The agent surface

Each wiki writes its own `llms.txt`, `pages.json`, `changes.json` and
`health.json` at its base, beside its search and graph catalogues, so a client
can be pointed at one wiki. The tree base keeps the aggregate `pages.json` and
writes an `llms.txt` that links to each wiki's file with its page count, rather
than listing every page in the tree. A per-wiki `changes.json` compares against
that wiki's previous render; the aggregate at the base compares against the
tree's. A root that is itself a wiki writes its own pair at the base, where it
covers everything below it. See [Agent access](../reference/agent-access.md).

## What it is not

Tags do not cross between wikis, and each tag page belongs to its own wiki. A
relative Markdown link can cross: from a wiki at the tree root
`[Beta](../beta/index.md)` opens the sibling page, and a page one section deep
uses `../../beta/index.md`. The aggregate graph records that link with
namespaced IDs at both ends; the wiki's own graph and Connections panel stay
inside the wiki. A link that leaves the tree is not a page link. Search and the
graph view span every wiki, with search results grouped by the wiki they came
from and graph nodes carrying the wiki they belong to. A tree is one place to
publish many wikis, not one wiki with many sections.
