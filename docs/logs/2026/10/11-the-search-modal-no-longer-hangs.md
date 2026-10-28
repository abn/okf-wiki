---
type: Log Entry
title: "The search modal no longer hangs on load"
description: "The search modal no longer hangs when a term is typed before the index has loaded."
tags: [log, update, fix]
kind: Update
status: stable
---

The search modal no longer hangs when a term is typed before the index has loaded. A caller that asked while the first fetch was in flight was resolved with a not-yet-loaded index, so `runSearch` called `fetchIndex` again from its own `.then`; both ran as microtasks, so the chain never yielded to the event loop and the request it waited on could never settle. The in-flight request is now shared, and a failed load leaves an empty catalogue rather than a null one. The same guard covers the `#tag` catalogue.
