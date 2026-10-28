---
type: Log Entry
title: "The search dialog keeps focus, names its input, and says what it found"
description: "The search dialog keeps focus, names its input, and says what it found."
tags: [log, update, fix]
kind: Update
status: stable
---

The search dialog keeps focus, names its input, and says what it found. The dialog declared `aria-modal`, so Tab was expected to stay inside it, and a walk of Tab presses on a freshly opened dialog left the results and reached the page chrome behind. The candidates are collected on each keypress, because the rows are rebuilt on every keystroke. The input was named by its placeholder, which is a hint rather than a name and disappears as soon as a reader types. Nothing was announced at all, so the result count, and in a tree the wikis it came from, now go to a polite status node beside the rows, clipped out of the layout rather than hidden. `reference/themes.md` says so.
