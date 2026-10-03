---
type: Decision
title: Why the watch re-renders the whole tree
description: Why a watched change rebuilds every wiki rather than the page that moved, and when that would be revisited.
tags: [adr, decision, watch]
status: stable
---

# 0004 The watch re-renders the whole tree

## Context

`serve --watch` polls the content directory and any layered theme once a second.
The poll hashes each file's path, size and modification time into one digest,
and any difference calls one render. That render is unconditional: it parses the
bundle again, and under `--multi-site` walks the tree and rebuilds every wiki,
each with its own pages, tag pages, theme assets and copied bundle files, and
then every group index.

The poll is also the coalescing window. The digest is taken once per tick, and
the render runs on the goroutine that reads it, so a burst of writes inside one
tick is a single render however many files moved, a render never overlaps the
poll or queues behind another, and a stream of changes that never stops renders
about once a second rather than once per change. The exception is a tree whose
render outlasts the interval, which renders back to back, still one at a time.

So a change anywhere re-renders everything, and the question this record answers
is whether it should become incremental instead.

Two things make the current shape defensible. The poll is cheap, because it
reads metadata rather than contents. And a full render cannot keep stale output:
a renamed section, a removed page or an outdated vendor bundle does not survive
a rebuild, and the outputs that are not pages, the navigation, the previous and
next chain, the search index and the tag pages, are derived from the whole
bundle rather than from one file.

Measured on the development machine, with the docs bundle as one wiki of 27
pages:

| tree | full re-render |
| --- | --- |
| 1 wiki, 27 pages | ~40 ms |
| 5 wikis, 135 pages | ~130 ms |
| 20 wikis, 540 pages | ~510 ms |

The poll costs about 3 ms over 540 files, roughly 0.3% of a core, and an idle
tree produces no renders at all. A burst of 200 or of 540 changed files produced
one render, and 50 changes spread over five seconds produced five or six.

## Decision

Keep the whole-tree re-render, and defer incremental rendering until a tree is
big enough that the reload is felt.

The threshold is not a page count but whether a re-render is noticeable against
the editing loop it serves. A few hundred pages is about half a second; that is
the scale to revisit, and not before.

## Consequences

A reload is always correct. There is no dependency graph to get wrong, no
per-file state to invalidate, and nothing to keep in step between a changed page
and the pages that link to it.

The cost is paid on every change and scales with the tree rather than the change.
For a bundle of the size okf-wiki targets that is tens of milliseconds and
invisible; a tree in the hundreds of pages would make it felt.

Two limits are worth stating. The digest is size and modification time, not
content, so a change that preserves both, which in practice means a hand-set
time, goes unnoticed. And a bundle that fails to parse mid-edit is rejected
before anything is rewritten, so the last good render keeps serving; a failure
later in a render would leave a partial output.

## Alternatives considered

**Re-render only the wiki that changed.** The natural middle step, rejected for
now because the non-page outputs still span the tree. The search index and the
tag pages are per wiki, but the group indexes, the tree-root assets and the
redirect are not. It removes the largest share of the work on a many-wiki tree
and adds a per-wiki digest and a mapping from a changed file back to its wiki.

**Re-render only the pages that changed.** The most work to reach and the most
to get wrong. The navigation, the previous and next chain, the tag pages and the
search index all depend on pages other than the one edited, so a page-level
invalidator has to track those dependencies as well. Not proportional to the win.

**Digest file contents rather than size and time.** It closes the hand-set time
case and costs a read of every file on every poll, which is the one thing the
poll avoids. The case is rare enough to leave open.

**Watch the filesystem rather than poll.** Rejected when the reload was built. A
bundle is small, an editor save arrives as several events that need coalescing,
and a poll needs no bookkeeping for the directories a new section introduces.

## See also

- `usage/cli.md` for the flag and what a reload does
- `usage/multi-site.md` for watching a tree of wikis
- `design/rendering.md` for the render itself
