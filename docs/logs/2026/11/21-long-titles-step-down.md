---
type: Log Entry
title: "Long titles step down one display size"
description: "A page title over 40 characters is set at a second, smaller display size, so it no longer wraps to three or four lines."
tags: [log, update, feature]
kind: Update
status: stable
---

A long page title, most often a log entry, wrapped to three lines at display size
on a desktop and four on a phone. A title over 40 characters now carries
`title-long` on its heading and is set at `--text-h1-long`, a new token that runs
from 26 to 36px against 36 to 48px for `--text-h1`, with tighter tracking and
1.15 leading. A title of 65 characters then takes two lines on a desktop and
three on a phone. Shorter titles are unchanged.

Templates gain a `{{.TitleLong}}` field. A theme that replaces `tokens.css`
without the new token keeps the full display size rather than losing the title's
size altogether. [Branding](../../../design/branding.md) and
[Themes](../../../reference/themes.md) describe both.
