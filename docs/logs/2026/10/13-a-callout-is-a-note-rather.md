---
type: Log Entry
title: "A callout is a note, not a landmark"
description: "A callout is a note rather than a landmark, and a table header says which direction it heads."
tags: [log, update, fix]
kind: Update
status: stable
---

A callout is a note rather than a landmark, and a table header says which direction it heads. A callout carried `role="region"`, so every card on a page added another entry to the landmark list, and its `aria-label` repeated the word the visible title in `.callout-title` already spells, which a screen reader announced twice. The role is now `note`, which is not a landmark, and the visible title is what names it. A table header was a bare `<th>`, so a screen reader read a cell as "Left Useful information" with nothing saying that Left is the column it sits under, and every header cell now carries `scope="col"`. The alignment a table declares is unchanged and a body cell gains no scope, since goldmark marks no cell as a row header. `reference/themes.md` says so.
