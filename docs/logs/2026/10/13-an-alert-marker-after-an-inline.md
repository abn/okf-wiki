---
type: Log Entry
title: "An alert marker after an inline node no longer becomes a callout"
description: "An alert marker after an inline node no longer becomes a callout."
tags: [log, update, fix]
kind: Update
status: stable
---

An alert marker after an inline node no longer becomes a callout. The probe that finds the marker read only text nodes, so a marker following a link, an image, a code span or emphasis looked like it opened the line: the callout card appeared with the marker stripped and only the inline node left as its body. The probe now stops at the first non-text node, and the space after the marker is skipped within its own node, so a space belonging to a later node survives. `design/rendering.md` says so, and `testbed/callouts.md` shows the markers that now stay blockquotes.
