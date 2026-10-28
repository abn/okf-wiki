---
type: Log Entry
title: "Added the --bundle archive download"
description: "With --bundle, every wiki writes okf-bundle.zip at its base, holding the source bundle under one folder: the Markdown with its frontmatter and every non-Markdown file, with dotfiles and dot-directories left out."
tags: [log, update, feature]
kind: Update
status: stable
---

With `--bundle`, every wiki writes `okf-bundle.zip` at its base, holding the source bundle under one folder: the Markdown with its frontmatter and every non-Markdown file, with dotfiles and dot-directories left out. Every page offers it from the header, and the narrowest widths move it to a labelled row in the drawer so the header keeps room for the brand; the graph view, which has no drawer, keeps the header control for a single bundle and a root-wiki tree and hides its own graph button instead, while a group-rooted tree has no archive there at all. Each wiki of a multi-site tree downloads its own, and a group index carries no control. The flag is off by default, because the archive is a second copy of the content in the output. The path is reserved while it is on, so a bundle that already holds that file fails the render with the file named. `design/rendering.md`, `design/branding.md`, `usage/cli.md`, `usage/container.md`, `usage/multi-site.md`, `usage/offline.md`, `usage/github-action.md`, `reference/configuration.md`, `reference/agent-access.md` and `reference/themes.md` say so.
