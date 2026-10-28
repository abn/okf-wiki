---
type: Log Entry
title: "A linked bundle's non-Markdown files reach the output"
description: "A linked bundle's non-Markdown files reach the output."
tags: [log, update, fix]
kind: Update
status: stable
---

A linked bundle's non-Markdown files reach the output. Both walks now resolve their root, since `WalkDir` treats a symlinked starting point as a file, so a wiki assembled by linking a checkout in served its pages with every image and download broken. `usage/multi-site.md` says so.
