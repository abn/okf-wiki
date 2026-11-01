---
type: Log Entry
title: "Documented page URLs on a static host"
description: "A page explains why page URLs keep the .html suffix, and what Cloudflare Pages, GitHub Pages and a plain file server do with them."
tags: [log, creation, docs]
kind: Creation
status: stable
---

Documented what a static host does with the page URLs a render writes. The
links keep their `.html` suffix, because `okf-wiki serve`, an offline folder and
a plain file server need the file name. Cloudflare Pages instead answers a
`.html` request with a 307 to the extensionless path and has no switch for it,
GitHub Pages serves both forms, and Workers static assets can serve the file
directly, though `html_handling = "none"` also stops a directory URL resolving
to `index.html`.

[Static hosting](../../../usage/hosting.md) is new, and
[Usage](../../../usage/index.md), [Command line](../../../usage/cli.md) and
[Configuration](../../../reference/configuration.md) link it.
