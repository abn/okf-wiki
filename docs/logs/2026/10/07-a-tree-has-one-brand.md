---
type: Log Entry
title: "A multi-site tree has one brand"
description: "A multi-site tree has one brand."
tags: [log, update, feature]
kind: Update
status: stable
---

A multi-site tree has one brand. The brand anchor targets the tree index through a new `{{.HomeHref}}`, which is `{{.Base}}` for a single bundle and the tree base for a tree page, so the mark and the wordmark mean the deployment home from any depth. The breadcrumb becomes a trail from the tree root down to the current wiki and page (`Wiki / Clients / Example / Operations / Guides / Setup`), with each folder linking to its group index and the wiki title linking to the wiki home; a group index ends at the group name and the tree root shows the title once. A tag page in a tree keeps that trail and then reads `Tags / #tag`, since it is a page of its own wiki. A single bundle's header and breadcrumb are byte for byte unchanged. `usage/multi-site.md`, `design/branding.md` and `reference/themes.md` say so.
