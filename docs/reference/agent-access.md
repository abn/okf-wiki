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
manifest is about 20 KB, while the HTML pages together are about 1.85 MB and the
Markdown sources about 169 KB. An agent should read `llms.txt` or `pages.json`
first, choose one page, and fetch that page's Markdown source. It should not
fetch the HTML for discovery.

## Multi-site and offline

In a tree the aggregate `pages.json` has namespaced IDs such as
`alpha:page:index` and carries `site` and `siteKey`. Links in both files follow
the render's URL mode: absolute paths online, and paths relative to the render
root for an offline folder.

The generated files reserve `llms.txt` and `pages.json` at the render root. A
bundle that already contains a file with either name fails the render and names
the collision rather than overwriting the reader's file.

The files are discovery metadata, not a second content store. The search
catalogue remains the place for ranking, and the graph catalogue remains the
place for links and tags.
