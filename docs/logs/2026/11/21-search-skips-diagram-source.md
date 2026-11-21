---
type: Log Entry
title: "Search snippets leave out diagram and chart source"
description: "The search index no longer holds the source of Mermaid and Vega-Lite fences, so a snippet never shows raw diagram markup."
tags: [log, update, fix]
kind: Update
status: stable
---

A search for "render" showed the Rendering pipeline page with a snippet that
ran into `flowchart LR A["content di…`, the source of the diagram below the
first paragraph. The indexed text now leaves out Mermaid and Vega-Lite fences,
which the page draws rather than shows. Ordinary code fences stay in, so a
command is still found by what it says.
[Rendering pipeline](../../../design/rendering.md) notes it under the index
step.
