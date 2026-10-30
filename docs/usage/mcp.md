---
type: Guide
title: MCP server
description: Run the read-only MCP server on the wiki's own listener, or standalone over stdio or HTTP, with bearer and federation examples.
tags: [usage, mcp, agents]
---

# MCP server

okf-wiki exposes a read-only MCP server in two shapes. `okf-wiki serve --mcp`
adds the endpoint to the wiki's own listener, which is the one-port setup for a
local wiki and for anything behind a reverse proxy. `okf-wiki mcp` is the
standalone command: it reads the published agent surface described in the
[agent access guide](agent-access.md), not the content directory, and can run
over stdio or its own HTTP port, locally or as a federator over published wikis.

Both shapes expose the same tools and the same result contract.

## Alongside the wiki

Add `--mcp` to `serve`:

```bash
okf-wiki serve --content ./docs --mcp
```

The endpoint is `http://127.0.0.1:8080/wiki/_/mcp`, under the render `--base`.
It shares the wiki's listener, its shutdown path and its render output, so
there is no second port to publish and no second process to supervise.

The `_/` segment is deliberate. With the default prefix the endpoint lives
under the base, beside the wiki, but outside the page namespace, so an ordinary
page, section or theme asset named `mcp` cannot shadow it. `<base>/_/` is then
reserved for server endpoints, not content. With the default base the URL is
`/wiki/_/mcp`; `--base /docs/` gives `/docs/_/mcp`, and `--base /` gives
`/_/mcp`.

Set `--mcp-prefix ""` to skip the segment and mount at `<base>/mcp` instead.
Any single segment of letters, digits, dot, dash or underscore works too, for
example `--mcp-prefix api` for `<base>/api/mcp`; leading and trailing slashes
around a segment are ignored. `OKF_WIKI_MCP_PREFIX` is the environment equivalent. It must be set to
an empty value, not left unset, to remove the prefix.

The endpoint's exact path and its trailing-slash form win over the file server,
so a section whose directory URL equals that path is answered by the endpoint
rather than its index. The `_/` default makes that collision unlikely. Without
it, a section named `mcp` at `<base>/mcp/` loses its index route; a custom prefix
like `api` can collide with a section directory `api/mcp/`.

Point a client at that URL:

```json
{
  "mcpServers": {
    "okf-wiki": {
      "url": "http://127.0.0.1:8080/wiki/_/mcp"
    }
  }
}
```

The source is the local render. It reloads before each tool call, so a
re-render appears without restarting the server, including a re-render from
`serve --watch`. A read that fails keeps the last good snapshot and retries
after 30 seconds rather than failing the call.

The endpoint has no authentication and no TLS, like the wiki beside it. It binds
loopback by default, and the startup banner names the URL. Publishing it means
choosing `--addr` and putting the wiki behind a proxy at the edge. In the
container, `OKF_WIKI_MCP=true` is the same as the flag.

## Quick start: a local render

Render the bundle, then point an MCP client at the output:

```bash
okf-wiki render --content ./docs --out ./public --base /wiki/
```

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

The local source reloads before each tool call, so a re-render is visible
without restarting the MCP process. A local source reads one render base; in a
multi-site tree that is the tree root. The per-wiki files are there to read one
wiki, not to split an offline folder into separate sources.

Start with `list_sources` or `context_for`. A typical tool sequence is:

```text
list_sources()
context_for(query="federated search", maxTokens=2000)
read_section(source="local", pageId="page:reference/agent-access", anchor="federating-sources")
read_page(source="local", pageId="page:reference/agent-access", maxTokens=4000)
```

Use the returned `url`, `markdownUrl`, `anchor` and `contentHash` as the
citation. `context_for` returns exact sections within a token budget, so it is
usually cheaper than reading whole pages.

## One published wiki

Use a source when the wiki is already published at a base URL:

```bash
export DOCS_TOKEN=...
okf-wiki mcp \
  --source docs=https://docs.example.com/wiki/ \
  --source-token docs=DOCS_TOKEN
```

```json
{
  "mcpServers": {
    "okf-wiki": {
      "command": "okf-wiki",
      "args": [
        "mcp",
        "--source", "docs=https://docs.example.com/wiki/",
        "--source-token", "docs=DOCS_TOKEN"
      ]
    }
  }
}
```

The source URL is the published wiki base, not the content directory. It may
sit under a different path from the render base. The search index carries the
render base, so citations still point at the right pages.

HTTPS is recommended but not required. HTTP is useful on a private network,
and a bearer token over plain HTTP prints a warning.

## Several wikis

Repeat `--source` to federate:

```bash
okf-wiki mcp \
  --source docs=https://docs.example.com/wiki/ \
  --source ops=http://ops.internal/wiki/ \
  --source-token ops=OPS_TOKEN
```

When more than one source is configured:

- Page IDs are prefixed, for example `docs:page:usage/cli`.
- Search results carry a `source` field.
- `search_wiki` and `context_for` search every source unless you pass
  `sources`.
- `suggest_pages` plans across every source unless you pass `sources`. Each
  candidate carries its `source`, and its `site` when the source is a tree.
- `read_page`, `read_section`, `neighbors`, `path`, `changes` and `health` take
  a `source`.
- `neighbors` and `path` stay inside one source.
- A tree can be one source at its base, which is the aggregate, or one source
  per wiki at that wiki's base. A per-wiki source reads that wiki's own
  `pages.json`, `changes.json` and `health.json`, which every render writes.
- `list_sources` shows each base, render ID and available state files.

A federated search example:

```json
{
  "query": "deployment",
  "sources": ["docs", "ops"],
  "limit": 5
}
```

If one source cannot answer a filtered search, the response includes its error
in `errors` and still returns results from the others.

## Remote MCP over HTTP

The standalone command can serve the same JSON-RPC handler over HTTP on its own
port at `/mcp`:

```bash
okf-wiki mcp --http 127.0.0.1:8090 \
  --source docs=https://docs.example.com/wiki/
```

Use this shape when one MCP process should federate several wikis, or when the
MCP listener needs a different network boundary from the wiki. For a single
local wiki, `serve --mcp` above is the shorter path: one process, one port and
the endpoint under the wiki base.

The HTTP listener has no built-in TLS or authentication. For a public
deployment, put it behind a rate-limiting proxy or CDN. For a private
deployment, use the existing Tailscale, OIDC or reverse-proxy boundary. A
Caddy example:

```caddyfile
docs-mcp.example.com {
  reverse_proxy 127.0.0.1:8090
}
```

MCP clients POST JSON-RPC to `https://docs-mcp.example.com/mcp`. A client that
supports remote MCP can point at that URL directly:

```json
{
  "mcpServers": {
    "okf-wiki": {
      "url": "https://docs-mcp.example.com/mcp"
    }
  }
}
```

TLS belongs at Caddy, not in the wiki renderer.

For an upstream source behind OAuth, put a client-credentials proxy or gateway
in front of that source and let it inject the `Authorization` header. A
browser-cookie OAuth flow does not work for headless MCP fetches.

## Common workflows

### Answer a question with citations

1. Call `context_for` with a token budget.
2. Read a returned section only if the excerpt is not enough.
3. Quote the text and cite the page URL, anchor and content hash.

A context pack can be truncated because of the budget or because the search
index caps one section at 12,000 bytes. Check `truncated` before treating the
pack as complete.

### Navigate the structural graph

Use `neighbors` to see a page's local graph and `path` to find a route between
two pages:

```json
{
  "source": "docs",
  "pageId": "page:usage/graph",
  "depth": 2,
  "kinds": ["link", "previous", "next"]
}
```

```json
{
  "source": "docs",
  "from": "page:usage/graph",
  "to": "page:reference/graph",
  "maxDepth": 6
}
```

`path` uses link edges. `previous` and `next` are page-order navigation and
come from `graph.json`. Tag hubs are not used to claim a page-to-page path.

### Plan a capture

`suggest_pages` ranks the subjects a draft would update, or answers that it
starts a new one. Call it before creating a page.

```json
{
  "text": "A draft, or a summary of the source being added",
  "limit": 5
}
```

The result carries `verdict` (`update` or `new_subject`), the threshold, the
matched words, and for each subject the page ID, role and Markdown URL to read
first. In a federation, a candidate carries its `source` and, for a tree, its
`site`; the verdict covers only the sources that read, and a source that could
not answer is named in `errors`. It is read-only and has no model in it;
[Plan a capture](similar.md) covers the scoring and the measured threshold.

### Check freshness and health

`changes` reports pages added, changed or removed in the latest render.
`health` reports broken links, orphans, tag-only pages, cycles, stale pages and
the most connected pages. A first render reports every page as added.

Long-lived clients should cache by `contentHash` and re-read only pages whose
hash changed.

## Troubleshooting

| Symptom | Check |
| :--- | :--- |
| Source fails with `401` or `403` | The bearer environment variable is set and the upstream expects that token. |
| Source loads no pages | The base URL points at the published wiki base, not the content directory. |
| Page IDs have no source prefix | Only one source is configured. Prefixes appear when there are two or more. |
| `read_page` fails but search works | `pages.json` or the Markdown source is missing at the source. |
| `changes` or `health` is unavailable | The source has no such file, or it is from another render. Re-render and retry. |
| A remote change is not visible | Remote sources refresh at most every 30 seconds. Local sources reload on each tool call. |
| The `serve --mcp` URL returns 404 | `--mcp` is set (or `OKF_WIKI_MCP=true`), and the path matches `--mcp-prefix`: `<base>/_/mcp` by default, `<base>/mcp` with an empty prefix. |
| HTTP client cannot connect | Use stdio for a local client, or point the client at the `/mcp` URL behind the proxy. |

## See also

- [Agent access](agent-access.md): the manifest, Markdown sources and citation
  guidance.
- [Agent access reference](../reference/agent-access.md): the manifest schema,
  tool arguments and result fields.
- [Graph view](graph.md): the human-facing graph controls.
- [Command line](cli.md): render, serve, search and MCP flags.
