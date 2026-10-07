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
A group has no assets of its own: its pages use the theme and an empty search
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

Each wiki has its own search index and tag catalogue, because each is a site of
its own. A group index is given an empty one, so the search control on a group
answers rather than failing; a search there finds nothing, since a group has no
pages.

## What it is not

Nothing crosses between wikis. A link inside one wiki resolves inside that wiki,
and there is no search or tag page spanning the tree. A tree is one place to
publish many wikis, not one wiki with many sections.
