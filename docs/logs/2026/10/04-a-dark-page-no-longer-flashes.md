---
type: Log Entry
title: "A dark page no longer flashes white between pages"
description: "A dark page no longer flashes white between pages."
tags: [log, update, fix]
kind: Update
status: stable
---

A dark page no longer flashes white between pages. The ground is painted on `html` as well as `body`, since the canvas a browser shows during a cross-document navigation is the root background, and the pre-paint restore sets `color-scheme` so the browser's default canvas is dark before the stylesheet applies. `reference/themes.md` says so.
