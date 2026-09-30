---
type: Guide
title: Blocks
description: Tables, task lists, footnotes, sources, and the three kinds of link a bundle can hold.
tags: [testbed, blocks]
sources:
  - id: okf-spec
    resource: https://github.com/GoogleCloudPlatform/knowledge-catalog
    title: The OKF specification
    author: team:knowledge-catalog
    last_modified: 2026-06-30T00:00:00Z
  - id: scope-example
    resource: all bundles rendered by this tool
    title: A scope descriptor, which is text rather than a destination
---

# Blocks

## Tables

Column alignment follows the delimiter row.

| Left | Centre | Right | Default |
| :--- | :----: | ----: | ------- |
| a | bb | ccc | dddd |
| longer cell | x | yyyy | z |
| `code in a cell` | **bold** | _italic_ | a [link](diagrams.md) |

A table wider than the reading column scrolls rather than reflowing, which is
the behaviour to check when restyling `table`.

## Task lists

- [x] Parse frontmatter
- [x] Assign heading ids
- [x] Build the search index
- [ ] Support nested task lists

The box is drawn by CSS from the `task-list-item` class, not by a Unicode
character, so a theme restyles it by restyling that class.

## Footnotes

A footnote reference[^1] sits inline, and the definition collects at the end of
the page[^2]. These two are ordinary footnotes: their labels match no
`sources[].id`, so they render exactly as goldmark writes them.

## Citing a source

A footnote whose label matches a `sources[].id` becomes a numbered citation to
the Sources section at the end of this page. That is how a claim points at the
material it came from.[^okf-spec]

The number follows the entry's position in `sources`, not the order the claims
appear, so a reference list reads the same however the prose is arranged. A
repeated citation reuses its number,[^okf-spec] and an entry no claim cites is
still listed, because the page declares it.

An entry whose `resource` is a scope descriptor rather than a destination
renders as text, since there is nothing to follow:[^scope-example] a page that
listed every bundle this tool renders has no URL to point at. The same applies
to a `last_modified` that is not a date: it is shown as written rather than
dropped.

[^1]: Footnotes come from the goldmark extension of the same name. The
    definition may wrap across lines.
[^2]: Search chunks are built from the AST, so footnote text is indexed as part
    of the section it appears in.
[^okf-spec]: The OKF specification
[^scope-example]: A scope descriptor, which is text rather than a destination

## Links

Three kinds, and each resolves differently.

**Inside the bundle.** [A sibling page](code.md) and a
[page in another section](../design/rendering.md) both become links to rendered
HTML, under the base path.

**With a fragment.** [Back to the top of this page](#blocks) keeps the anchor
and points at the heading id rather than the file.

**Outside the bundle.** A relative link that climbs out of the content
directory cannot become a page, because the renderer only turns files it read
into HTML. Given a `--repo` root it becomes a `/repo/` link instead; without one
it is left exactly as written, which is why there is no live example of that
form here. This bundle is published without a repository root, and an
unresolvable link is worse than no example. See
[Links](../design/rendering.md#links).

## Images and other files

A bundle is allowed to carry files that are not Markdown: a diagram exported
once, a screenshot, a fixture. Every non-Markdown file is copied into the site
under the same path it has in the bundle, and a link to it resolves like a link
to a page does. Dotfiles and dot-directories are skipped, so a bundle that is
also a working tree cannot publish its `.git` or its `.env`.

![The four stages of a render](assets/pipeline-stages.svg)

An image is resolved the same way, so this is a working image and not a
placeholder: the SVG sits at `testbed/assets/pipeline-stages.svg` in the bundle
and is served from the matching path in the site. A theme that restyles prose
can leave this alone; it inherits the surrounding text colour through the SVG's
own fills.

A non-Markdown file inside the bundle, linked like any other. It is copied into
the site at the same path, so this resolves to a real file:

[the bundle's own log](../log.md) and a non-page file,
[the render pipeline diagram as raw SVG](assets/pipeline-stages.svg).

## Escaping inline code

Backticks, `*asterisks*` and other markup characters are inert inside a code
span. So are pipes inside a table cell, which is the one case where the
surrounding block would otherwise steal them.
