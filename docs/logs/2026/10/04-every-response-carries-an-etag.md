---
type: Log Entry
title: "Every response carries an ETag"
description: "Every response carries an ETag from the file's bytes beside Last-Modified, so a re-render, which moves every file's modification time, no longer turns an unchanged asset's next request into a full 200. design/rendering.md says so."
tags: [log, update, fix]
kind: Update
status: stable
---

Every response carries an `ETag` from the file's bytes beside `Last-Modified`, so a re-render, which moves every file's modification time, no longer turns an unchanged asset's next request into a full `200`. `design/rendering.md` says so.
