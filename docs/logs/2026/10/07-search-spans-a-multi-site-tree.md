---
type: Log Entry
title: "Search spans a multi-site tree"
description: "Search spans a multi-site tree."
tags: [log, update, feature]
kind: Update
status: stable
---

Search spans a multi-site tree. The tree writes one aggregate `search-index.json` at its base holding the entries of every wiki that rendered, each labelled with the wiki it came from, and a page fetches it through a new `{{.SearchBase}}` / `window.__WIKI_SEARCH`. The client groups results under a heading per wiki and adds an All wikis / This wiki toggle. A single bundle's index and the `#tag` path are unchanged. `usage/multi-site.md`, `design/rendering.md`, `design/branding.md` and `reference/themes.md` say so.
