---
type: Log Entry
title: "The sidebar nests every directory level"
description: "The sidebar nests a directory at every level, so the change log reads 2026, then its month, then the entries."
tags: [log, update, feature]
kind: Update
status: stable
---

The rail grouped a section by its first directory only, so every page under
`logs/2026/09/` and `logs/2026/10/` sat in one flat `2026` group. Each directory now
nests inside its parent, and the disclosure for the current page opens at every
level above it. Groups follow directory order, so a month does not move when an
entry title changes, and each level opens one group at a time.

Each month directory carries an `index.md` whose heading names the month, so the
rail reads `2026`, then `October`, then the entries rather than `10`. The month page
is its group's heading link, and the entry pages below it are the history.

The breadcrumb no longer links a section without an `index.md` at a page the render
never writes; the section title is plain text there, as it already was in the rail.
[`design/rendering.md`](../../../design/rendering.md) and
[`reference/themes.md`](../../../reference/themes.md) record the structure and the
disclosure names a theme sees.
