---
type: Log Entry
title: "The MCP server adds neighbors and path"
description: "The MCP server adds neighbors and path."
tags: [log, update, feature]
kind: Update
status: stable
---

The MCP server adds `neighbors` and `path`. Neighbours return the local graph with `link`, `tag`, `previous` and `next` edges; previous and next are derived from page order in `pages.json` inside each site and section. Paths are undirected and carry the edge that connects each pair. The agent page documents the filters and depth limits.
