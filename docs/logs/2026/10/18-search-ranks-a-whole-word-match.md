---
type: Log Entry
title: "Search ranks whole-word matches higher"
description: "Search ranks a whole-word match above one found mid-word, and a term an entry repeats in its text above a single mention, on top of the existing title, document, section and content weights."
tags: [log, update, feature]
kind: Update
status: stable
---

Search ranks a whole-word match above one found mid-word, and a term an entry repeats in its text above a single mention, on top of the existing title, document, section and content weights. Equal scores break on global keys, an overview entry before a section entry, then the shorter title, then the earlier index. The set of matching entries, the grouping and the caps are unchanged. `design/rendering.md` says so.
