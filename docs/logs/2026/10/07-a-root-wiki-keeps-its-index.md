---
type: Log Entry
title: "A root wiki keeps its own search index"
description: "A multi-site content root that is itself a wiki keeps its own search-index.json at the base instead of having it replaced by the labelled aggregate, so its search reads as a single bundle's with no per-wiki headings. design/rendering.md and usage/multi-site.md say so."
tags: [log, update, fix]
kind: Update
status: stable
---

A multi-site content root that is itself a wiki keeps its own `search-index.json` at the base instead of having it replaced by the labelled aggregate, so its search reads as a single bundle's with no per-wiki headings. `design/rendering.md` and `usage/multi-site.md` say so.
