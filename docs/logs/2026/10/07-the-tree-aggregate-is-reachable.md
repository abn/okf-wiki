---
type: Log Entry
title: "The tree aggregate is reachable from a group"
description: "The tree aggregate is reachable from a group index, so a search there finds hits anywhere in the tree rather than only the wikis below it."
tags: [log, update, docs]
kind: Update
status: stable
---

The tree aggregate is reachable from a group index, so a search there finds hits anywhere in the tree rather than only the wikis below it. Each wiki's own index and tag catalogue sit beside the tree aggregate. `usage/multi-site.md` and `adr/0004-watch-rerenders-the-whole-tree.md` say so.
