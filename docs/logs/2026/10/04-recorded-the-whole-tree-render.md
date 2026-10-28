---
type: Log Entry
title: "Recorded the whole-tree re-render decision"
description: "Added adr/0004-watch-rerenders-the-whole-tree.md, which records that serve --watch re-renders every wiki on any change rather than the page that moved, the measured cost at 1, 5 and 20 wikis, and the decision to defer incremental rendering until a tree is big enough to feel it. usage/cli.md and adr/index.md link to it."
tags: [log, creation]
kind: Creation
status: stable
---

Added `adr/0004-watch-rerenders-the-whole-tree.md`, which records that `serve --watch` re-renders every wiki on any change rather than the page that moved, the measured cost at 1, 5 and 20 wikis, and the decision to defer incremental rendering until a tree is big enough to feel it. `usage/cli.md` and `adr/index.md` link to it.
