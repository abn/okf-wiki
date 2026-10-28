---
type: Log Entry
title: "The renderer warns on long page titles"
description: "The command warns on stderr when a page title is longer than 80 characters or carries a line break, names the first ten and counts the rest, and still succeeds."
tags: [log, update, render]
kind: Update
status: stable
---

The command warns on stderr when a page title is longer than 80 characters or carries a line break, names the first ten and counts the rest, and still succeeds. The title names the page in the sidebar, the graph, the browser tab, and a search result, so `usage/cli.md` and `usage/graph.md` send the fix to the bundle: shorten the title and move the summary into `description`.
