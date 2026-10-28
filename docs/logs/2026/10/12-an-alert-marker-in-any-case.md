---
type: Log Entry
title: "An alert marker in lower or mixed case renders as its callout"
description: "An alert marker in lower or mixed case renders as its callout."
tags: [log, update, fix]
kind: Update
status: stable
---

An alert marker in lower or mixed case renders as its callout. The marker was matched as written, so `> [!warning]` fell through to a plain blockquote with the marker visible as text. GitHub matches it without regard to case, and the renderer now does too, so every spelling of the five kinds gives the same card. A marker outside those five is still left alone. `design/rendering.md` says so, and `testbed/callouts.md` shows the lower and mixed case forms.
