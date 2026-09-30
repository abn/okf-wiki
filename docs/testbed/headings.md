---
type: Guide
title: Headings
description: Table of contents depth, and what happens when two headings slug to the same id.
tags: [testbed, headings]
---

# Headings

Heading text becomes the anchor id, lowercased, with spaces and punctuation
folded to hyphens. Ids are made unique within a page. Every level-two heading
grows a `#` permalink on hover, and the table of contents lists level two only,
so it stays short on a page with deep nesting.

## Level two

A level two heading is the usual case. Hover it to see the permalink.

## Level three

A level three heading still gets an id and an anchor, so it can be linked
directly, but it stays out of the table of contents.

### A level three heading

This one is linkable but not listed above.

#### A level four heading

The same rule applies further down.

##### And level five

Deeper still. Every heading is an anchor target; only level two is navigation.

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

The first "Setup" takes the bare id.

## Setup 2

## Setup

A third "Setup". Its id has to step past both `setup-2` and `setup-2-2`, so
the sequence is not simply setup, setup-2, setup-3.

## Setup 2

A second "Setup 2" has the same base slug as the first, so it takes its own
suffix.
