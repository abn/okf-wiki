---
type: Log Entry
title: "Multi-site follows a directory symlink"
description: "--multi-site follows a directory symlink, including a content directory that is itself a symlink, and serve --watch sees a change inside a linked bundle, so a tree can be assembled by linking bundles in from their checkouts. usage/multi-site.md says so, including that a directory already walked is not walked again and that a container render needs the link targets mounted at the same absolute paths."
tags: [log, update]
kind: Update
status: stable
---

`--multi-site` follows a directory symlink, including a content directory that is itself a symlink, and `serve --watch` sees a change inside a linked bundle, so a tree can be assembled by linking bundles in from their checkouts. `usage/multi-site.md` says so, including that a directory already walked is not walked again and that a container render needs the link targets mounted at the same absolute paths.
