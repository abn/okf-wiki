---
type: Log Entry
title: "MCP federates published wikis"
description: "okf-wiki mcp can federate one or more published wikis by base URL, with optional per-source bearer tokens."
tags: [log, update, feature]
kind: Update
status: stable
---

`okf-wiki mcp` can federate one or more published wikis by base URL, with optional per-source bearer tokens. The render base and render ID travel in `search-index.json`, so a source mounted under a different path still reads Markdown and cites the right files. Multi-source page IDs are namespaced, search and context merge across sources, and graph navigation is source-scoped. `--http` exposes the same JSON-RPC handler at `/mcp` for a remote deployment behind a proxy. `reference/agent-access.md` and `usage/cli.md` record the source contract.
