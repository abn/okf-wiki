---
type: Guide
title: Blocks
description: Tables, task lists, footnotes, and the three kinds of link a bundle can hold.
tags: [testbed, blocks]
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
the page[^2].

[^1]: Footnotes come from the goldmark extension of the same name. The
    definition may wrap across lines.
[^2]: Search chunks are built from the AST, so footnote text is indexed as part
    of the section it appears in.

## Links

Three kinds, and each resolves differently.

**Inside the bundle.** [A sibling page](code.md) and a
[page in another section](../design/rendering.md) both become links to rendered
HTML, under the base path.

**With a fragment.** [Back to the top of this page](#blocks) keeps the anchor
and points at the heading id rather than the file.

**Outside the bundle.** A relative link that climbs out of the content
directory cannot become a page, because the renderer only turns files it read
into HTML. Given a `--repo` root it becomes a `/repo/` link instead; without
one it is left exactly as written. There is deliberately no live link of that
kind here, because this bundle is published without a repository root and an
unresolvable link is worse than no example. See
[Links](../design/rendering.md#links).

## Escaping inline code

Backticks, `*asterisks*` and other markup characters are inert inside a code
span. So are pipes inside a table cell, which is the one case where the
surrounding block would otherwise steal them.
