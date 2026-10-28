---
type: Log Entry
title: "The search client prepares the index once"
description: "The search client lowercases each entry's match fields once as it scans the index and debounces typing by a short pause, so a burst of keystrokes triggers one search over prepared text rather than re-lowercasing the whole index on every keystroke."
tags: [log, update, perf]
kind: Update
status: stable
---

The search client lowercases each entry's match fields once as it scans the index and debounces typing by a short pause, so a burst of keystrokes triggers one search over prepared text rather than re-lowercasing the whole index on every keystroke. Enter, the arrows and Escape stay immediate, and the arrows and Enter flush a pending search first so they act on the current query. Opening the modal or finishing the index load with a query already present still shows results without another keystroke. Ranking, result order and the DOM are unchanged. `design/rendering.md` says so.
