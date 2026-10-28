---
type: Log Entry
title: "A tree skips a wiki it cannot read"
description: "A wiki of a tree that cannot be read is skipped and reported instead of failing the whole render, so one stale link in a linked checkout no longer takes every wiki offline."
tags: [log, update, fix]
kind: Update
status: stable
---

A wiki of a tree that cannot be read is skipped and reported instead of failing the whole render, so one stale link in a linked checkout no longer takes every wiki offline. The skipped wiki leaves no output and no catalog card. `design/rendering.md` says so.
