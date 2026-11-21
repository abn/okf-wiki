---
type: Log Entry
title: "No lead when it would repeat the title"
description: "The paragraph under the title renders only for a description that differs from it, and the meta description falls back to the first paragraph."
tags: [log, update, feature]
kind: Update
status: stable
---

A page with no description showed its title twice, once as the heading and again
as the lead under it, as on the home page and a month index. The lead now renders
only when the page has a description and it differs from the title, ignoring case
and outer space.

The meta description still always has a value, but without a description it is
now the first body paragraph, cut to 160 characters at a word boundary, and only
then the title. Search engines get a real summary instead of the heading again.

Templates gain a `{{.Lead}}` field for the paragraph under the title.
`{{.Description}}` keeps feeding the meta description, so an existing theme keeps
working. [Branding](../../../design/branding.md) and
[Themes](../../../reference/themes.md) list both fields.
