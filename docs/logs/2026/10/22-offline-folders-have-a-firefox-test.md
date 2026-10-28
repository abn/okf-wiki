---
type: Log Entry
title: "Offline folders are covered by a Firefox file:// smoke test in CI"
description: "Offline folders are covered by a Firefox file:// smoke test in CI."
tags: [log, update, verification]
kind: Update
status: stable
---

Offline folders are covered by a Firefox `file://` smoke test in CI. It checks the relative base, search and tags, Mermaid, the graph and the root redirect. No renderer change was needed; `usage/offline.md` and `contribution/index.md` say so.
