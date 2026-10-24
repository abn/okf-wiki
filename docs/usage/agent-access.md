---
type: Guide
title: Agent access
description: The published files an agent reads, how to choose and cite a page, and where the MCP server fits.
tags: [usage, agents, mcp]
---

# Agent access

Every render publishes files that an agent can read without crawling HTML.
They are described in full in the [agent access
reference](../reference/agent-access.md), and the [MCP server](mcp.md) can
serve them over MCP.

## The published surface

| File | What it gives an agent |
| :--- | :--- |
| `llms.txt` | A short, sectioned index of pages with links to their Markdown. |
| `pages.json` | Title, description, IDs, URLs, tags, hashes and heading anchors. |
| Markdown sources | The page text beside the rendered HTML. |
| `search-index.json` and `search-content.json` | The browser search catalogue. |
| `graph.json` | Pages, tags, links and page-order edges. |
| `changes.json` | Pages added, changed or removed in the latest render. |
| `health.json` | Broken links, orphans, tag-only pages, cycles, stale pages and hubs. |

`llms.txt` and `pages.json` are written at the render root. A tree writes one
aggregate pair at its base. Inline or offline renders keep the same files.

## Choose a page

Start with the discovery files, then fetch only what you need:

```bash
curl -s https://docs.example.com/wiki/llms.txt
curl -s https://docs.example.com/wiki/pages.json
```

`llms.txt` is enough to choose a page by title. `pages.json` is better when the
agent needs IDs, tags, status, dates, hashes or heading anchors. Each page
entry names both `url` and `markdownUrl`. Fetch `markdownUrl` for the source and
`url#anchor` for a rendered section.

A page hash changes when its source changes, so a cache can re-read only pages
whose `contentHash` moved.

## Search from a terminal

For a local render, `okf-wiki search` returns ranked snippets without starting
MCP:

```bash
okf-wiki render --content ./docs --out ./public --base /wiki/
okf-wiki search "federated sources" --out ./public --base /wiki/ --json --limit 5
```

The JSON result carries `pageId`, `anchor`, `url`, `markdownUrl`,
`contentHash` and an extractive `snippet`. `--site`, `--tag` and `--section`
narrow it. The CLI and MCP use the same field weights for a flat result list;
the browser adds per-wiki grouping and caps.

## Cite accurately

Use the stable fields rather than reconstructing them:

- `pageId` identifies the graph page, namespaced in a tree aggregate.
- `anchor` identifies the heading inside the page.
- `url` opens the rendered section.
- `markdownUrl` opens the source.
- `contentHash` checks that a cached copy is current.

An empty `anchor` means the page-level overview, not a missing section.

## MCP

The MCP server adds the same search plus section reads, budgeted context,
neighbours, paths, changes and health. See [MCP server](mcp.md) for local,
published, federated and HTTP setup, tool examples and troubleshooting.

The reference documents every tool argument and result field in [Agent access
reference](../reference/agent-access.md).

## Multi-site and offline

In a tree, `pages.json` and `llms.txt` are aggregates at the tree base. Page IDs
carry the wiki path, for example `alpha:page:index`. A single bundle writes the
same files at its own base.

Offline renders use URLs relative to the render root and embed the search and
graph data as scripts. The Markdown sources remain beside the pages, so an
offline folder is still readable by an agent.

## See also

- [MCP server](mcp.md): running the server and federating sources.
- [Agent access reference](../reference/agent-access.md): schema and tool contract.
- [Graph view](graph.md): the human-facing graph.
- [Command line](cli.md): render, serve and search.
