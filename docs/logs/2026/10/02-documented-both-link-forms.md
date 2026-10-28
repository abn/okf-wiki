---
type: Log Entry
title: "Documented both in-bundle link forms"
description: "design/rendering.md documents both in-bundle link forms, relative to the page and from the bundle root, and testbed/blocks.md carries a bundle-relative link."
tags: [log, update]
kind: Update
status: stable
---

`design/rendering.md` documents both in-bundle link forms, relative to the page and from the bundle root, and `testbed/blocks.md` carries a bundle-relative link. A root-relative link now resolves from the bundle root rather than from the page, so it survives the page moving.
