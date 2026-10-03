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

So a change anywhere re-renders everything, and the question this record answers
is whether it should become incremental instead.

Two properties of the loop bound the damage. The poll is the coalescing window:
the digest is taken once per tick and the render runs on the goroutine that
reads it, so a burst of writes inside one tick is a single render however many
files moved, renders never overlap or queue, and a stream of changes that never
stops renders about once per interval rather than once per change. The exception
is a tree whose render outlasts the interval, which renders back to back, still
one at a time. And the built tree is swapped into place rather than written over
the live output, so a reader during a re-render sees the old tree or the new one
and never a file missing.

Measured on the development machine, with the docs bundle as one wiki of 27
pages:

| tree | full re-render |
| --- | --- |
| 1 wiki, 27 pages | ~40 ms |
| 5 wikis, 135 pages | ~130 ms |
| 20 wikis, 540 pages | ~510 ms |

The poll costs about 3 ms over 540 files, roughly 0.3% of a core, and an idle
tree produces no renders at all. A burst of 200 or of 540 changed files produced
one render, and 50 changes spread over five seconds produced five or six. Before
the swap, twelve renders under a hammering client produced a handful of 404s and
500s, a request landing in the window where the output directory had been
emptied; after it, zero across twenty renders.

## Decision

Keep the whole-tree re-render, and defer incremental rendering until a tree is
big enough that the reload is felt.

The threshold is not a page count but whether a re-render is noticeable against
the editing loop it serves. A few hundred pages is about half a second; that is
the scale to revisit, and not before.

Two smaller decisions come with it. The interval is a flag, `--watch-interval`
(or `OKF_WIKI_WATCH_INTERVAL`), default one second, because it is a real
preference: how soon an edit appears against how much work a burst of saves
causes. And the render builds beside the output and swaps it in, so a live-viewed
storm cannot serve a file that is missing.

## Consequences

A reload is always correct. There is no dependency graph to get wrong, no
per-file state to invalidate, and nothing to keep in step between a changed page
and the pages that link to it.

The cost is paid on every change and scales with the tree rather than the change.
For a bundle of the size okf-wiki targets that is tens of milliseconds and
invisible; a tree in the hundreds of pages would make it felt.

The swap means the output directory is replaced, not updated, so the directory
inode changes on every render. A reader holding an open directory listing across
a swap keeps the old tree until it is removed a moment later; nothing depends on
the inode staying the same, and the server reads from disk per request.

Three limits are worth stating. The digest is size and modification time, not
content, so a change that preserves both, in practice a hand-set time, goes
unnoticed. A bundle that fails to parse mid-edit is rejected before the swap, so
the last good render keeps serving. And a failure after the swap has begun is
narrow: the build happens first, and the two renames that follow are the only
steps that touch the live tree.

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

**Write into the live output directory rather than swapping.** What the first
implementation did, and it exposes a window where a page is absent: an unlucky
request gets a 404 or Go's `Error reading directory`, and a large file could be
read truncated. The swap removes the window for two renames and a temporary
directory, at the cost of the output directory's inode changing on every render.

## See also

- `usage/cli.md` for the flag and what a reload does
- `usage/multi-site.md` for watching a tree of wikis
- `design/rendering.md` for the render itself
