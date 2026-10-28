---
type: Log Entry
title: "okf-wiki search returns cited snippets"
description: "okf-wiki search returns ranked, extractive snippets from a rendered site as human text or JSON, with page ID, anchor, Markdown URL and content hash. okf-wiki mcp exposes the same search as the read-only search_wiki MCP stdio tool."
tags: [log, update, feature]
kind: Update
status: stable
---

`okf-wiki search` returns ranked, extractive snippets from a rendered site as human text or JSON, with page ID, anchor, Markdown URL and content hash. `okf-wiki mcp` exposes the same search as the read-only `search_wiki` MCP stdio tool. Search reuses the browser index and joins `pages.json` for citations. Search extraction also keeps spaces at soft line breaks and between adjacent links, so snippets and phrase matches read correctly. `reference/agent-access.md` and `usage/cli.md` record the commands.
