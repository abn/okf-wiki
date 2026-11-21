---
type: Log Entry
title: "A touch footer for search"
description: "On a touch device the search modal drops its keyboard hints, keeps its actions as 44px pills, and closes from a Cancel button."
tags: [log, update, fix]
kind: Update
status: stable
---

On a phone the search modal showed hints for the arrow keys, Enter and Escape
across two footer rows, for keys the reader does not have. On a touch device the
hints are gone, and the footer holds only its actions, Open in graph and the
wiki scope of a tree, as 44px pills. With no action to show, the footer is
hidden. The ESC chip in the header becomes a Cancel text button in the accent
green, in a 44px hit area. The switch is a coarse pointer with no hover rather
than a width, since a narrow desktop window still has a keyboard.

The browser's own clear button in the search field, which painted in the
platform blue, is redrawn in the theme's faint ink on every device.
[Themes](../../../reference/themes.md) lists the new `.search-close-text` label.
