---
type: Log Entry
title: "graph.json carries page-order edges"
description: "graph.json now carries previous and next structural edges for page order inside a section."
tags: [log, update, feature]
kind: Update
status: stable
---

`graph.json` now carries `previous` and `next` structural edges for page order inside a section. The browser drawing continues to draw links and tags only; the agent graph tools use the catalogue edges. `reference/graph.md`, `reference/agent-access.md`, `usage/graph.md` and `design/graph-evolution.md` record it.
