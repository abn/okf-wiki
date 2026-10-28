---
type: Log Entry
title: "A group index of a tree keeps its base"
description: "A group index of a tree published under a non-root base no longer points at theme assets and an empty search catalogue written elsewhere."
tags: [log, update, fix]
kind: Update
status: stable
---

A group index of a tree published under a non-root base no longer points at theme assets and an empty search catalogue written elsewhere. A group index renders with the tree's own base, so its assets and catalogue now sit under the base directory beside it instead of at the output root, which the two paths share only when the base is `/`. `usage/multi-site.md` says so.
