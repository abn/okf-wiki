---
type: Log Entry
title: "Every wiki writes its own agent files"
description: "Every wiki of a multi-site tree writes its own llms.txt, pages.json, changes.json and health.json at its base, so a client can be pointed at one wiki and read its pages without loading the tree."
tags: [log, update, feature]
kind: Update
status: stable
---

Every wiki of a multi-site tree writes its own `llms.txt`, `pages.json`, `changes.json` and `health.json` at its base, so a client can be pointed at one wiki and read its pages without loading the tree. The tree base keeps the aggregate `pages.json` and its `llms.txt` becomes a router: one line per wiki with a page count and a link to that wiki's file, a few kilobytes where the whole-tree listing grew with every page. A per-wiki `changes.json` diffs against that wiki's previous render, and a per-wiki `health.json` names its own pages. A reserved filename in any wiki's bundle now fails the tree render, not only a file at the root, and the message names the directory. `reference/agent-access.md`, `usage/agent-access.md`, `usage/multi-site.md` and `usage/mcp.md` record it.
