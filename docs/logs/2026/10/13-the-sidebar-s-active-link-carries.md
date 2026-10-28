---
type: Log Entry
title: "The sidebar's active link carries aria-current"
description: "The sidebar's active link carries aria-current beside the active class, and the labels the default theme drew in uppercase text are small caps now."
tags: [log, update, feature]
kind: Update
status: stable
---

The sidebar's active link carries `aria-current` beside the active class, and the labels the default theme drew in uppercase text are small caps now. The class only painted the position, so a screen reader had nothing to announce for the page a reader was on. An uppercase text transform reaches a screen reader as uppercase text, so a label such as "Previous" was announced shouted, while small caps keep the same weight and leave the text as it was written. `reference/themes.md` says so.
