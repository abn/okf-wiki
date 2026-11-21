---
type: Log Entry
title: "Log entries carry their date and walk the log by time"
description: "A log entry shows its date after the type, and its Older and Newer cards follow the entries in date order."
tags: [log, update, feature]
kind: Update
status: stable
---

A log entry now shows its date in the row under the title, straight after the
type. The date comes from an optional `date` field, or from the entry's
`logs/YYYY/MM/DD-` path when there is none.

The cards at the end of an entry used to follow the rail, so they jumped across
months in no time order. They now read Older and Newer, each with a short date,
and walk every entry by date across months and years. Two entries on one day
order by file name. [Change log](../../../log.md) describes the field and the
order.
