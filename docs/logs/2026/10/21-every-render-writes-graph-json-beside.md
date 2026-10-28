---
type: Log Entry
title: "Every render writes graph.json beside the search catalogue"
description: "Every render writes graph.json beside the search catalogue."
tags: [log, update, feature]
kind: Update
status: stable
---

Every render writes `graph.json` beside the search catalogue. It holds the structural graph of the bundle: one node per page and per tag, one edge per in-bundle Markdown link and per frontmatter tag, and the heading anchor and link text that place a link in its section. An in-bundle link whose target the bundle does not contain becomes a stub node with no URL. A multi-site tree writes a file per wiki and one aggregate at its base with every node ID prefixed by the wiki's path. `reference/graph.md`, `design/rendering.md`, `usage/cli.md` and `usage/multi-site.md` say so.
