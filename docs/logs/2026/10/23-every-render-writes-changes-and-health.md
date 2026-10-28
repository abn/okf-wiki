---
type: Log Entry
title: "Every render writes changes.json and health.json"
description: "Every render writes changes.json and health.json."
tags: [log, update, feature]
kind: Update
status: stable
---

Every render writes `changes.json` and `health.json`. Changes compare the current page manifest with the previous render by content hash. Health reports broken links, orphans, tag-only pages, link cycles, stale pages and the most connected pages. The MCP server exposes both as `changes` and `health`. It reloads the rendered files before each tool call, treats missing or malformed change and health files as unavailable rather than fatal, and marks index-capped sections as truncated.
