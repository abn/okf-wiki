---
type: Log Entry
title: "serve --watch takes an interval"
description: "serve --watch takes an interval, --watch-interval (or OKF_WIKI_WATCH_INTERVAL, default 1s), and renders into a directory built beside the output that is swapped into place, so a page loaded during a re-render is never missing or half-written. usage/cli.md, reference/configuration.md and adr/0004-watch-rerenders-the-whole-tree.md carry it."
tags: [log, update]
kind: Update
status: stable
---

`serve --watch` takes an interval, `--watch-interval` (or `OKF_WIKI_WATCH_INTERVAL`, default `1s`), and renders into a directory built beside the output that is swapped into place, so a page loaded during a re-render is never missing or half-written. `usage/cli.md`, `reference/configuration.md` and `adr/0004-watch-rerenders-the-whole-tree.md` carry it.
