---
type: Log Entry
title: "The force layout is deterministic"
description: "The force layout seeds its collision tie-breaks from the visible catalogue, so the same graph and view settings produce the same positions on every visit."
tags: [log, update, feature]
kind: Update
status: stable
---

The force layout seeds its collision tie-breaks from the visible catalogue, so the same graph and view settings produce the same positions on every visit. The section grid is fixed already. Positions remain browser-side and are not added to `graph.json`. `usage/graph.md`, `reference/graph.md`, `design/rendering.md` and `design/graph-evolution.md` say so.
