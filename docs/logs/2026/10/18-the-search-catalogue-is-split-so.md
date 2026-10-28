---
type: Log Entry
title: "Split the search catalogue for first paint"
description: "The search catalogue is split so the page text no longer holds up the first result. search-index.json keeps the page table and each entry's heading, section and anchor; the entry texts move to the companion search-content.json, aligned by position."
tags: [log, update, perf]
kind: Update
status: stable
---

The search catalogue is split so the page text no longer holds up the first result. `search-index.json` keeps the page table and each entry's heading, section and anchor; the entry texts move to the companion `search-content.json`, aligned by position. The client searches titles, documents and sections as soon as the light index arrives, fetches the texts behind it, joins them onto the entries, and re-runs an open query so content matches appear without a keystroke. A tree writes the same pair per wiki and an aggregate pair at its base. `design/rendering.md`, `usage/multi-site.md` and `usage/cli.md` say so.
