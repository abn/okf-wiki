---
type: Log Entry
title: "Graph names stay visible at full zoom"
description: "A node's name is no longer dropped for covering its own node once the graph is zoomed in past about twice its size."
tags: [log, update, fix]
kind: Update
status: stable
---

Fixed names disappearing from the graph view at full zoom. A name sits a small
gap above its node, and the gap shrinks as the view zooms in. Past about twice
the natural size it was smaller than the padding the overlap test adds around
a node, so every name was judged to cover its own node and was dropped, hubs
included. The overlap test now skips the node a name belongs to, which is what
[Graph view](../../../usage/graph.md) already described: a name is drawn where
it does not cover another name or another node.
