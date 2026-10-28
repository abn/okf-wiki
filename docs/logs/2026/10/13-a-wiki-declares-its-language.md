---
type: Log Entry
title: "A wiki declares the language of its pages"
description: "A wiki declares the language its pages are written in. <html lang=\"en\"> was hardcoded, so a screen reader and a read-aloud voice picked English whatever language the bundle was written in."
tags: [log, update, feature]
kind: Update
status: stable
---

A wiki declares the language its pages are written in. `<html lang="en">` was hardcoded, so a screen reader and a read-aloud voice picked English whatever language the bundle was written in. A `--lang` flag and its `OKF_WIKI_LANG` environment variable set it, defaulting to `en`, and the root redirect carries the same language as the pages it redirects to. A value that is not a language tag is refused, since a browser ignores a tag it cannot read and the page would then read as English whatever was asked for. `reference/configuration.md` says so.
