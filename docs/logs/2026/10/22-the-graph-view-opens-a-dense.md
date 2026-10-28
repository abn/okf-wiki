---
type: Log Entry
title: "The graph opens a readable patch"
description: "The graph view opens a dense catalogue on a readable patch around a page with a typical number of connections rather than fitting every node."
tags: [log, update, fix]
kind: Update
status: stable
---

The graph view opens a dense catalogue on a readable patch around a page with a typical number of connections rather than fitting every node. Selecting a node fades the rest, frames its neighbourhood when that fits, and lists its connections in the panel. Names keep their screen size, are dropped where they would cover another name or node, and edges are culled off-screen and hidden at overview zoom unless a node is selected. `usage/graph.md` and `design/rendering.md` say so.
