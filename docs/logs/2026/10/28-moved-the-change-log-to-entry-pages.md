---
type: Log Entry
title: "Moved the change log to entry pages"
description: "Moved the change log from one file to an entry page per change under logs/."
tags: [log, update]
kind: Update
status: stable
---

`log.md` held every change since the bundle was created. Each change is now one page
under `logs/YYYY/MM/`, named by the day and a slug, with the original text and its
links rebased for the page depth, and its first sentence as the description.
`log.md` is now a static page that explains the model and lists no entries; nothing
regenerates it. [`contribution/index.md`](../../../contribution/index.md) says how to
add one.
