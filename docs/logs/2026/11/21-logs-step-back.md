---
type: Log Entry
title: "Log entries and log-kind tags step back by default"
description: "The graph opens without log entries whenever a bundle has them and another type, and tag pages move log-kind tags into a Change log row."
tags: [log, update, feature]
kind: Update
status: stable
---

Changed how the change log shows up outside its own section. The graph view
opened on every page in a single bundle, so the log entries filled the map as
a dot grid and their kind tags crowded the tag hubs. It now opens on Content
pages (no logs) whenever the catalogue has log entries and another type, not
only in a multi-site tree. The status line counts the hidden entries and
offers a Show log entries pill, which turns into Hide log entries once they
are shown. [Graph view](../../../usage/graph.md) describes the control.

On tag pages, tags that label a kind of change rather than a topic leave the
All tags cloud for a Change log row under it, sorted by count, and a topic
tag's Often with leaves them out. They are detected, not declared: a tag counts
when only log entries carry it and at least a tenth of them do.
[Rendering pipeline](../../../design/rendering.md#tags) records the rule.
