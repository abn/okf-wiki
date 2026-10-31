---
type: Log Entry
title: "Planned a capture before writing it"
description: "Added okf-wiki similar and the MCP suggest_pages tool, which rank the existing subjects a draft should update or say it starts a new one."
tags: [log, creation, feature]
kind: Creation
status: stable
---

`okf-wiki similar` reads a bundle and ranks the subjects a draft belongs to, so
a capture can edit an existing page instead of creating a near duplicate. The
MCP server exposes the same planner as `suggest_pages`. Both are read-only and
have no model in them: the same draft against the same bundle gives the same
answer on every machine.

The rank is idf-weighted coverage over a page's title, description, tags,
headings, path and body text. A subject's score is its best page's, and a deep
dive's `overview`, `report` and `evidence` pages collapse into one candidate. Log
entries are skipped because a log repeats the words of every subject it touched,
and bare numbers are skipped because a year belongs to one page rather than to a
subject.

The threshold was measured before it was fixed. Replaying 25 real edits against
the commit before each edit found the edited subject first in 82 percent of
subject pages and in the top three in 94 percent. Edits to a section `index.md`
were scored separately and did much worse, which is expected of a hub. On 10 new
subjects it left 7 below the threshold, and the rank is more reliable than the
verdict at the boundary. The replay is in `scripts/evaluate-similar.py`, and the
default is documented as per convention rather than universal.

The planner only suggests. A wrong merge is worse than a new page, so nothing is
written and the caller decides.

[Plan a capture](../../../usage/similar.md) documents the flags, the JSON and the
verdict. [MCP server](../../../usage/mcp.md) records the `suggest_pages` call.
[Capability candidates](../../../design/capability-candidates.md) marks the item
shipped and sets the next slice.
