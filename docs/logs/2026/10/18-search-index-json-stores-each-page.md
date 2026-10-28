---
type: Log Entry
title: "The search index stores each page once"
description: "search-index.json stores each page's url and titles once in a page table that its entries refer to by index, instead of repeating them on every section."
tags: [log, update, perf]
kind: Update
status: stable
---

`search-index.json` stores each page's url and titles once in a page table that its entries refer to by index, instead of repeating them on every section. The client expands the table back into the entries it scores, so results are unchanged. A chunk's text is capped at 12,000 bytes, cut at a word boundary where the prefix holds one and at a rune boundary otherwise, so one long section cannot dominate the file. A tree's aggregate merges the wikis' page tables and rebases each entry onto the merged one. `design/rendering.md` and `usage/multi-site.md` say so.
