---
type: Log Entry
title: "Stylesheets load in parallel from the head"
description: "The theme's three stylesheets are linked from the head rather than @import-ed into one, so they load in parallel instead of as a serial chain, and an index.html request is served in place rather than answered with a 301 to ./, so an index link does not cost a redirect before the document. design/rendering.md says so."
tags: [log, update, fix]
kind: Update
status: stable
---

The theme's three stylesheets are linked from the head rather than `@import`-ed into one, so they load in parallel instead of as a serial chain, and an `index.html` request is served in place rather than answered with a 301 to `./`, so an index link does not cost a redirect before the document. `design/rendering.md` says so.
