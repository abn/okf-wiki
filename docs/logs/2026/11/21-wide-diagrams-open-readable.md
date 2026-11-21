---
type: Log Entry
title: "Wide diagrams open at a readable size"
description: "A diagram opens at no less than 85% of its natural size and scrolls sideways when wider than its card, instead of shrinking to fit."
tags: [log, update, feature]
kind: Update
status: stable
---

Changed how a wide diagram opens. A diagram was fitted to its card, so a wide
flowchart drew its 14px labels at under half size on a desktop and about a
quarter on a phone. It now opens at the larger of its fit scale and 85%, never
above natural size, and a drawing wider than the card scrolls sideways from its
left edge. A fade marks each edge that hides more drawing, and the full screen
button picks up an "Expand" label while the drawing overflows.

The percentage button switches between that readable start and the whole
drawing fitted to the card. On a phone the zoom buttons are left out, since
pinch zoom works in full screen, and copy and expand are 44px targets.
[Rendering pipeline](../../../design/rendering.md) describes the card.
