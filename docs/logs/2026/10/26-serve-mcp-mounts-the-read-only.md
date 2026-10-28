---
type: Log Entry
title: "serve --mcp mounts the MCP server"
description: "serve --mcp mounts the read-only MCP server on the wiki's own listener, at <base>/_/mcp (/wiki/_/mcp by default), so one process and one port publish the pages and the agent surface."
tags: [log, update, feature]
kind: Update
status: stable
---

`serve --mcp` mounts the read-only MCP server on the wiki's own listener, at `<base>/_/mcp` (`/wiki/_/mcp` by default), so one process and one port publish the pages and the agent surface. It uses the local render and reloads before each tool call, so it follows `--watch`. The `_/` segment keeps the endpoint outside the page namespace; `--mcp-prefix ""` removes it and mounts at `<base>/mcp`, and any other single segment moves it. `OKF_WIKI_MCP=true` is the container equivalent, and the startup banner names the URL. The standalone `okf-wiki mcp --http` remains for federation and a separate network boundary. `usage/mcp.md`, `usage/cli.md`, `usage/container.md`, `reference/configuration.md`, `reference/agent-access.md`, `compose.yaml` and `README.md` record it.
