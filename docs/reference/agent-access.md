---
type: Reference
title: Agent access
description: The llms.txt discovery file and pages.json manifest that let an agent choose a page before fetching its Markdown source.
tags: [reference, agents, metadata]
---

# Agent access

Every render writes two small discovery files beside the search and graph
catalogues: `llms.txt` and `pages.json`. They describe the content pages and
point at the Markdown source that is already copied beside each page. Neither
file contains page text, so an agent can choose what to read before paying for
the content.

A single bundle writes both files at its base. A multi-site tree writes one
aggregate pair at the tree base. A wiki below the tree root does not write its
own pair, because the aggregate spans every wiki. A root wiki is a tree of one
and writes the pair at its own base.

## llms.txt

`llms.txt` is a Markdown discovery document. It starts with the wiki title and
description, links to `pages.json`, and then groups one line per page by site
and section:

```markdown
# Wiki

> Docs for the example service.

Machine-readable page list: [pages.json](pages.json).

## Usage

- [Command line](/wiki/usage/cli.md): Running the binary, with render and serve.
- [Configuration](/wiki/usage/configuration.md): Every flag and its default.
```

Each link points at the page's Markdown source, not its HTML. The description
comes from the page's frontmatter and is omitted when empty. In a tree, a `##`
heading names each wiki and the section headings move down one level.

## pages.json

`pages.json` is the machine-readable page table. It has a schema version, the
render date, and one entry per content page:

```json
{
  "version": 1,
  "generatedAt": "2026-10-23",
  "pages": [
    {
      "id": "page:usage/cli",
      "title": "Command line",
      "description": "Running the binary, with render and serve.",
      "url": "/wiki/usage/cli.html",
      "markdownUrl": "/wiki/usage/cli.md",
      "section": "Usage",
      "tags": ["usage", "cli"],
      "status": "stable",
      "contentHash": "…",
      "headings": [
        {"id": "live-reload", "title": "Live reload"}
      ]
    }
  ]
}
```

| Field | Meaning |
| :--- | :--- |
| `id` | The graph page node ID. An aggregate prefixes it with the wiki's path. |
| `title` | The page title. |
| `description` | The frontmatter description, when present. |
| `url` | The rendered HTML page. |
| `markdownUrl` | The Markdown source beside it. |
| `section` | The section title. |
| `site`, `siteKey` | The wiki title and path, only in a tree aggregate. |
| `type`, `status` | Frontmatter values, when present. |
| `tags` | The page's frontmatter tags. |
| `generatedAt`, `staleAfter` | Frontmatter dates, when present. |
| `contentHash` | SHA-256 of the page's Markdown source, including frontmatter. |
| `headings` | Heading anchors and titles, in document order. |

The `headings` list uses the same anchors as the rendered page, so an agent can
cite `url#anchor` or fetch the Markdown and find the section by its heading.
`contentHash` changes when the source changes, which lets a cache decide whether
to re-read a page without comparing the whole text.

## Token use

A manifest entry is a few hundred bytes. The whole current documentation
manifest is a few tens of kilobytes, while the HTML pages together are over a
megabyte and the Markdown sources are a few hundred kilobytes. An agent should
read `llms.txt` or `pages.json` first, choose one page, and fetch that page's
Markdown source. It should not fetch the HTML for discovery.

## Search from a terminal

Search reads the rendered catalogue, not the source bundle, so render once and
then query as often as needed:

```bash
okf-wiki search "cross-wiki" --out ./public --base /wiki/ --json --limit 5
```

`--site`, `--tag` and `--section` narrow the result. `--limit` is 1 through 20
and defaults to 5. The JSON response is the same shape the MCP tool returns:

```json
{
  "query": "cross-wiki",
  "total": 6,
  "results": [
    {
      "pageId": "page:usage/graph",
      "title": "Graph view",
      "pageTitle": "Graph view",
      "section": "Usage",
      "url": "/wiki/usage/graph.html",
      "markdownUrl": "/wiki/usage/graph.md",
      "tags": ["usage", "graph"],
      "snippet": "… a cross-wiki edge in the aggregate …",
      "score": 44,
      "contentHash": "…"
    }
  ]
}
```

`total` is the number of matches after filters, before `--limit`. `snippet` is
an extractive window around the first matching term. `pageId`, `anchor`,
`markdownUrl` and `contentHash` are the citation fields. The CLI and MCP share
one result order. They use the same field weights and tie breaks as the browser
search for a flat result list; the browser additionally groups a tree's results
by wiki and applies its own per-site caps.

## MCP search and reading

`okf-wiki mcp` serves search and reading as a read-only Model Context Protocol
stdio server:

```json
{
  "mcpServers": {
    "okf-wiki": {
      "command": "okf-wiki",
      "args": ["mcp", "--out", "/srv/wiki/public", "--base", "/wiki/"]
    }
  }
}
```

The server exposes eight read-only tools:

| Tool | Arguments | Returns |
| :--- | :--- | :--- |
| `search_wiki` | `query`, `limit`, `site`, `tag`, `section` | Ranked snippets with citations. |
| `read_page` | `pageId`, `maxTokens` | The page's Markdown source, capped at the budget. |
| `read_section` | `pageId`, `anchor`, `maxTokens` | One heading-anchored section as plain text. |
| `context_for` | `query`, `maxTokens`, `limit`, filters | The best sections that fit the budget. |
| `neighbors` | `pageId`, `depth`, `kinds` | The local graph with typed edges. |
| `path` | `from`, `to`, `maxDepth`, `kinds` | The shortest undirected path and its edges. |
| `changes` | none | Pages added, changed or removed in the latest render. |
| `health` | none | Broken links, orphans, tag-only pages, cycles, stale pages and most connected pages. |

`read_page` defaults to 4,000 tokens, `read_section` to 2,000 and `context_for`
to 3,000. The limit is an estimate of four bytes per token, not a model
tokenizer. Each result carries `estimatedTokens` and `truncated` so an agent
knows when to ask for a smaller section or a larger budget.

`context_for` is the usual first call for a task. It runs the ranked search,
selects the matching sections rather than whole pages, and truncates the last
section that would cross the budget. The response sets `truncated` when the
budget or the 12,000-byte index cap shortened any section. The text is
extractive; no summary is generated.

`neighbors` accepts `link`, `tag`, `previous` and `next` in `kinds`. Previous
and next are stored in `graph.json` from the page order inside each site and
section; the agent reader also derives them from `pages.json` for older
catalogues. `path` uses the same undirected graph, so it can follow links, tags
or page order. A path result includes the edge that connects each pair.

`changes` reads `changes.json`, which compares the current `pages.json` with the
previous render in the output directory. It reports `added`, `changed` and
`removed` pages with the current and previous content hashes. Only the latest
render is stored; a first render reports every page as `added`. `health` reads
`health.json`, which reports broken links, orphans, tag-only pages, link cycles,
stale pages and the ten most connected pages. Index pages are not orphans.
Stale dates are parsed as RFC 3339, like the tag badge.

`search_wiki` works without `pages.json`, with the Markdown URL derived from the
HTML URL. Page IDs, `read_page`, `read_section`, the tag filter and the site
filter need `pages.json`. `neighbors` and `path` work from `graph.json`;
new catalogues store previous and next there, while an older catalogue without
them uses `pages.json`. `read_page` also needs `--base` when
the render uses absolute URLs, so the server can resolve a Markdown URL back to
the site directory. The server reloads the rendered files before each tool call,
so a re-render is visible without restarting the MCP process. If `changes.json`
or `health.json` is absent or unreadable, only those two tools fail; search and
reads still work.

## Multi-site and offline

In a tree the aggregate `pages.json` has namespaced IDs such as
`alpha:page:index` and carries `site` and `siteKey`. Links in both files follow
the render's URL mode: absolute paths online, and paths relative to the render
root for an offline folder.

The generated files reserve `llms.txt`, `pages.json`, `changes.json` and
`health.json` at the render root. A bundle that already contains one of those
names fails the render and names the collision rather than overwriting the
reader's file.

`llms.txt` and `pages.json` are discovery metadata. `changes.json` and
`health.json` are render state, not a second content store. The search
catalogue remains the place for ranking, and the graph catalogue remains the
place for links and tags.
