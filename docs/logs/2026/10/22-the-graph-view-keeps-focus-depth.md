---
type: Log Entry
title: "The graph view keeps its state in the URL"
description: "The graph view keeps focus, depth, Show, Filter and Tags in the query string and restores them on back and forward."
tags: [log, update, feature]
kind: Update
status: stable
---

The graph view keeps focus, depth, Show, Filter and Tags in the query string and restores them on back and forward. Tags are a real filter: repeated `tag` parameters narrow to pages carrying every named tag, and the tag list marks a tag the graph does not contain so it can be removed. Typing in Filter replaces the current history entry; discrete controls add one. `usage/graph.md`, `reference/graph.md`, `reference/themes.md`, `design/rendering.md`, `usage/offline.md` and `design/graph-evolution.md` say so.
