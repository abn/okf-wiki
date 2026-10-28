---
type: Log Entry
title: "The MCP server adds read tools"
description: "The MCP server adds read_page, read_section and context_for alongside search_wiki."
tags: [log, update, feature]
kind: Update
status: stable
---

The MCP server adds `read_page`, `read_section` and `context_for` alongside `search_wiki`. Reads carry page IDs, anchors, Markdown URLs and content hashes, and cap their text at an estimated token budget. The agent page documents the read tools and the budget defaults.
