---
type: Log Entry
title: "Release Please manages the version from Conventional Commits"
description: "Release Please manages the version from Conventional Commits."
tags: [log, update, release]
kind: Update
status: stable
---

Release Please manages the version from Conventional Commits. It opens a release pull request that bumps `Version` in `cmd/okf-wiki/main.go` and adds the changelog; merging it tags `vX.Y.Z`, dispatches the image publish at that tag, and runs the SemVer checker, which points the action's `vX` and `vX.Y` refs at the newest patch. `contribution/index.md` and `usage/github-action.md` record it.
