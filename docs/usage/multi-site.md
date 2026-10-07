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

## Naming a group

An entry on a group index is named from, in order:

- a `.meta.json` in the directory, `{"title": ..., "description": ...}`
- the frontmatter of the entry's own `index.md`, when the entry is a wiki
- a `README.md`, its first heading and its first line of prose
- the directory name, title-cased

None is required. A folder still lists, named after its directory, so a tree
needs no metadata to work.

## Watching

`serve --multi-site --watch` re-renders the whole tree when anything under it
changes: an edit to a page, a page added, or a new wiki directory. The change is
live on the next page load. See [Live reload](cli.md#live-reload).

## The search index

Search spans the whole tree. Each wiki keeps its own `search-index.json`, so it
remains a site of its own, and after every wiki has rendered the tree writes one
aggregate index at its base. Every wiki and group fetches that aggregate, so a
term that appears in a sibling wiki is found from here and answers with a
heading naming the wiki it came from. A wiki that could not be read contributes
nothing to it.

The modal can be narrowed to the current wiki with the All wikis / This wiki
toggle in its footer. The toggle appears only in a tree, where the aggregate
lives at the tree base rather than the wiki's own base; on a single wiki the two
are the same and it is hidden.

A group index has no pages, so a search run from one finds hits in the wikis
below it. The tag catalogue stays per wiki, and `#tag` searches the current
wiki's tags, because a tag page is a page of the wiki that carries the tag.

## What it is not

Links and tags do not cross between wikis. A link inside one wiki resolves
inside that wiki, and each tag page belongs to its own wiki. Search is the
exception: one index at the tree base spans every wiki, with results grouped by
the wiki they came from. A tree is one place to publish many wikis, not one wiki
with many sections.
