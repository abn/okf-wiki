---
type: Log Entry
title: "A rendered diagram has an accessible name and is announced as one image"
description: "A rendered diagram has an accessible name and is announced as one image."
tags: [log, update, feature]
kind: Update
status: stable
---

A rendered diagram has an accessible name and is announced as one image. The SVG had nothing for a screen reader to announce, and a reader view read the drawing as a run of node labels. The name comes from the fence's own `accTitle` and `accDescr` when the author wrote them, and from the nearest preceding heading otherwise, the type word is the readable one rather than mermaid's internal name, and the drawing is one image rather than every shape in it. `design/rendering.md` says so.
