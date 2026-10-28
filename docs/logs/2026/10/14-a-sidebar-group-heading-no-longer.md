---
type: Log Entry
title: "Aligned a sidebar group heading with its siblings"
description: "A sidebar group heading no longer sits a step deeper than the loose pages and leaf groups beside it."
tags: [log, update, fix]
kind: Update
status: stable
---

A sidebar group heading no longer sits a step deeper than the loose pages and leaf groups beside it. The group's `summary` carries the row's horizontal padding, and the link inside it also picked up the generic `.side li a` padding, so a second-level group such as a section's sub-directory read as a third-level subitem of the page above it. The inner link now drops that padding, so the group stays aligned with its siblings while its own pages still hang off the hairline rule. `reference/themes.md` says so.
