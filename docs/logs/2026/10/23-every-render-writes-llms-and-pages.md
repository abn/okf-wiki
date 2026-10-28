---
type: Log Entry
title: "Every render writes llms.txt and pages.json for agent discovery"
description: "Every render writes llms.txt and pages.json for agent discovery."
tags: [log, update, feature]
kind: Update
status: stable
---

Every render writes `llms.txt` and `pages.json` for agent discovery. They list page titles, descriptions, Markdown URLs, stable IDs, headings, tags and source hashes without carrying page text. A tree writes one aggregate pair at its base. `reference/agent-access.md`, `usage/cli.md` and `design/rendering.md` record the files.
