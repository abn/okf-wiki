---
type: Log Entry
title: "Search ranks pages before the change log"
description: "Search results put matching pages first and log entries in a capped Change log group after them, and each row leads with its title."
tags: [log, update, feature]
kind: Update
status: stable
---

Log entries repeat the words of every page they touch, so a query such as
"render" returned five log entries before the Rendering pipeline page. When a
single bundle's results include both pages and log entries, they now come in
two groups, Pages and then Change log. The log group holds at most three rows,
and its heading counts the rest and links to the `#log` tag page. A tree keeps
its groups by wiki and sorts each wiki's log entries after its pages.

Each row leads with its title, followed by a line naming the section, the
section and page for a heading inside a page, or the date for a log entry. The
snippet stops after two lines, and the empty state suggests fewer words, a page
title, or `#` for tags, above the section links.
[Rendering pipeline](../../../design/rendering.md) describes the ranking.
