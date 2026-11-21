---
type: Log Entry
title: "A header for the nav drawer"
description: "The navigation drawer opens with a sticky header holding the brand and a close button, which takes focus when the drawer opens."
tags: [log, update, feature]
kind: Update
status: stable
---

The navigation drawer had no header and no close button, so the only ways out
were the backdrop and the menu button under the scrim. It now opens with a 64px
header that stays in place while the navigation scrolls: the brand mark and
wordmark on the left, linking home, and a 44px close button on the right, named
"Close navigation". The close button comes first in the page, so opening the
drawer puts focus on it. Escape and the backdrop still close the drawer.

The header shows wherever the sidebar is a drawer: at 860px and below, and in
the full width reading mode. [Branding](../../../design/branding.md) and
[Themes](../../../reference/themes.md) describe it, with the new `drawerClose`
id.
