---
type: Index
title: Test bed
description: Every construct the renderer supports, on one set of pages, so a theme author or contributor can see the whole surface at once.
tags: [testbed]
---

# Test bed

These pages exercise every construct the renderer supports. They exist for two
reasons: a theme author needs to see the whole rendering surface in one place
without assembling their own bundle, and every rendering change needs a
regression target that is visible rather than asserted only in a unit test.

Nothing here is documentation about the renderer itself. That is
[`design/rendering.md`](../design/rendering.md). This is the output of it.

| Page | What it covers |
| :--- | :------------ |
| [Callouts](callouts.md) | All five GitHub alert levels, plus a plain blockquote |
| [Blocks](blocks.md) | Tables, task lists, footnotes, links |
| [Code](code.md) | Highlighting across languages, and unknown languages |
| [Diagrams](diagrams.md) | Mermaid flowcharts, sequence diagrams, state machines |
| [Headings](headings.md) | Table of contents depth, repeated and colliding headings |
| [First run](nested/setup/first-run.md) | A page two directories deep, and links out of it |

## How to use it

Render this bundle and open the section:

```bash
okf-wiki render --content docs --out .scratch/wiki
okf-wiki serve --content docs --out .scratch/wiki --addr 127.0.0.1:8080
```

Then visit `/wiki/testbed/`. The section order comes from the default
alphabetical append, so it sits after `reference` and `usage`.
