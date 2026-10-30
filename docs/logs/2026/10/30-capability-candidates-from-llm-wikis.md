---
type: Log Entry
title: "Recorded capability candidates from LLM-maintained wikis"
description: "Added a working list of candidates drawn from the LLM Wiki pattern: a citation surface, wiki health, page planning, retrieval receipts, rules at the point of use, a human digest, and a later semantic layer."
tags: [log, creation, design]
kind: Creation
status: stable
---

[Capability candidates](../../../design/capability-candidates.md) records what
the LLM Wiki pattern suggests for okf-wiki, with the evidence for each item and
the smallest version worth shipping. It is a working list beside
[Graph evolution](../../../design/graph-evolution.md), not a commitment.

The list keeps the render read-only. The five findings that survive that
constraint are that structural checks belong to code, that an agent needs a
manifest before it reads, that the compiled page is not the retrieval layer,
that a rule works at the point of use, and that near-duplicate pages are the
default failure of an ingestion loop.

A read-only probe over two bundles set the order. The docs bundle has 145 pages,
one with `sources`, none with `verified` and one using footnotes; a research
bundle has 673 pages, 455 with `sources`, 15 with `verified`, 435 with footnotes
and 17 pages whose footnotes have no source entry. The same probe found that a
title-similarity check alone flags 1,019 pairs in the research bundle, almost all
of them `overview`, `report` and `evidence` siblings of one subject, so a
duplicate check needs the subject and role rather than the title.

The assessment behind this page is in the investigations wiki, at
`research/investigations/methods/llm-wiki/`.
