---
type: Guide
title: Callouts
description: All five GitHub alert levels, and how a plain blockquote differs.
tags: [testbed, callouts]
---

# Callouts

A blockquote whose first line begins with an alert marker becomes a callout
card. The marker is stripped, so the body is the sentence that follows it.

## The five levels

> [!NOTE]
> Useful information that users should know, even when skimming.

> [!TIP]
> Helpful advice for doing things better or more easily.

> [!IMPORTANT]
> Key information users need to know to achieve their goal.

> [!WARNING]
> Urgent info that needs immediate attention, or a warning about possible
> problems.

> [!CAUTION]
> Advises about risks or negative outcomes of certain actions.

## Marker only

The marker does not need trailing text on its own line. This is a single
paragraph:

> [!NOTE] A note whose marker shares the line with its body.

## With a body

> [!WARNING]
> A callout can hold any block content.
>
> - A list
> - Across
> - Several items
>
> ```go
> fmt.Println("and a code block")
> ```

## Not a callout

A blockquote without a marker stays a blockquote. So does one whose marker is
not at the very start of the first line, and one using a marker that is not a
recognised level.

> An ordinary blockquote, for comparison.

> [!UNKNOWN]
> An unrecognised level is left alone.

>Text with no space after the marker is not a callout either.
