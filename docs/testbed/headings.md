---
type: Guide
title: Headings
description: Table of contents depth, and what happens when two headings slug to the same id.
tags: [testbed, headings]
---

# Headings

Heading text becomes the anchor id, lowercased, with spaces and punctuation
folded to hyphens. The table of contents is built from levels two and three, so
it stays short on a page with deep nesting.

## Level two

A level two heading is the usual case.

## Level three

A level three heading appears in the table of contents as an indented child.

### Level four

A level four heading gets an id and an anchor, but stays out of the table of
contents, which is why this sentence is not listed above.

## Repeated headings

Two headings with the same text are common in a page that documents several
variants of the same thing. The second must not reuse the first one's id, or
both table of contents entries jump to the same place.

## Repeated headings

The heading immediately above is a deliberate repeat. It is the regression
target for anchor collisions, so its id must differ from the first
"Repeated headings".

## A heading that collides with a numbered one

Here the collision is subtler. A plain "Setup" slugifies to `setup`, and a
"Setup 2" also slugifies to `setup-2`, which is exactly the id the second
"Setup" above was given. Getting this right means checking every id the page
has already emitted, not counting repeats of one base slug.

## Setup

## Setup 2

## Setup

A third "Setup", to confirm the sequence stays unique end to end.
