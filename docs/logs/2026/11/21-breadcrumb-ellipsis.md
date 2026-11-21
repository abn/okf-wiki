---
type: Log Entry
title: "The last breadcrumb ends in an ellipsis"
description: "A breadcrumb wider than its bar shortens the current page's segment with an ellipsis instead of clipping it mid-letter."
tags: [log, update, fix]
kind: Update
status: stable
---

On a phone the current page's breadcrumb segment was cut at the edge of the bar
partway through a letter, so "Rendering pipeline" read as "Rendering pipel". The
segment now gives way first and ends in an ellipsis, while the links before it
keep their full width. The page title below still names the page in full.
[Themes](../../../reference/themes.md) notes the behaviour beside the breadcrumb
markup.
