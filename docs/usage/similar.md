---
type: Guide
title: Plan a capture
description: Rank the existing pages a draft should update, or learn that it starts a new subject, before writing it into the bundle.
tags: [usage, similar, agents, capture]
---

# Plan a capture

A wiki grows by edits to pages that already exist, not by a new page beside
them. `similar` answers the question that decides which one happens: given a
draft, which existing subject does it belong to, and if none is close, is it a
new subject?

It reads the bundle, not a render, so it runs before anything is written. It is
read-only and has no model in it: the same draft against the same bundle gives
the same answer on every machine.

## Run it

```bash
okf-wiki similar --content ./docs --text draft.md
okf-wiki similar --content ./docs --text - < draft.md
okf-wiki similar --content ./docs --top 3 "graph diff between two renders"
```

| Flag | Meaning |
| :--- | :--- |
| `--content DIR` | The OKF bundle to plan against. Auto-detected when omitted. |
| `--text FILE` | The draft, or `-` for standard input. Words on the command line are used when `--text` is absent. |
| `--top N` | Subject candidates to return, 1 through 20. Default 5. |
| `--min-score F` | Update threshold. `0` uses the default. |
| `--base PATH` | URL prefix for the links in the response. Default `/wiki/`. |
| `--json` | Write the full response as JSON. |

The human output names the verdict, then each subject with the page to read
first and the stems that matched:

```text
Update: the best subject matches at 0.453, above the 0.160 threshold.

1. Graph evolution
   design/graph-evolution  0.453
   read: /wiki/design/graph-evolution.md
   matched stems: diff, graph, render
```

## What it does with a draft

A draft is tokenised, stop words and bare numbers are dropped, and the common
plural and verb endings are folded so that `renders` finds `render`. Each word
is weighted by how rare it is in the bundle, so `graphml` counts for more than
`render`.

A page is scored on its title, description, tags, headings, path and body text.
A subject's score is its best page's score, and a deep dive's `overview`,
`report` and `evidence` pages collapse into one candidate. The result is a
coverage figure between 0 and 1: 1.0 means every distinctive word in the draft
appears in a page title, and a lower figure means less of the draft is
accounted for.

Two exclusions keep the rank honest. Log entries are skipped, because a log
repeats the words of every subject it touched and would match almost any draft.
Bare numbers are skipped, because a year or a count belongs to one page rather
than to a subject.

## The verdict

`update` means the best subject meets the threshold. `new_subject` means it does
not, and the draft probably belongs in a page of its own.

The default is `0.16`, measured rather than chosen: replaying 25 real edits from
a deep-dive bundle against the commit before each edit found the edited subject
first in 82 percent of subject pages and in the top three in 94 percent. Edits
to a section `index.md` were a separate row and scored much worse, which is
expected: a hub is a list of links, not a subject. On 10 genuinely new subjects
it left 7 below the threshold.

The rank is more reliable than the verdict. A new subject can reach 0.17 on a
shared generic word while a real edit can sit at 0.14, so read the candidate
list even when the verdict says `new_subject`. Run the evaluation in
`scripts/evaluate-similar.py` on a bundle with a different page convention
before trusting the number. Usage: `evaluate-similar.py --repo DIR --bundle
PATH`.

## From an agent

The MCP server exposes the same planner as `suggest_pages`:

```json
{
  "text": "A draft or a summary of the new source",
  "limit": 5
}
```

The response carries the verdict, the threshold, the matched words, and for each
subject the page ID, role, Markdown URL and content hash to read first. See
[MCP server](mcp.md) for the client setup.

The intended use is one line at the start of a capture: plan the draft, then
update the returned pages or accept that it is a new subject. A wrong merge is
worse than a new page, which is why the planner only suggests and never writes.

## Limits

- It ranks words, not meaning. Two pages about the same idea in different
  vocabulary will not match, and a shared rare word can pull a draft toward the
  wrong subject at low scores.
- Section index pages are lists of links, so an edit to one ranks poorly. That
  is the one row in the measurement that scored badly on purpose.
- The body text is the search catalogue's chunks, capped at 12 KiB per chunk
  (one heading block), so a very long page is ranked on what its index holds.
- It reads one bundle. For a tree of wikis, point the MCP tool at the tree or at
  one wiki, where a candidate also carries its `site`.
- The threshold is per convention. A bundle whose pages are one paragraph each
  will separate subjects differently from a bundle of deep dives.
