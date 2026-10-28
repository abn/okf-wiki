---
type: Log Entry
title: "Releases publish after their archives"
description: "A release is created as a draft, gets its archives, and is published only once the upload is verified."
tags: [log, update, fix]
kind: Update
status: stable
---

The release pipeline publishes the release last now. Release Please creates each
release as a draft with its tag, `binaries.yml` attaches the six archives and
`checksums.txt` and checks every file against the build, and a separate job
publishes the draft, which is the point the tag and the assets become immutable.
The semver check and the image dispatch run after publication.

Publishing first, as the release did before, fails the archive upload on an
immutable release with `HTTP 422`, which is how 1.1.0 came to be published without
archives. That release cannot be repaired, so the next one carries them.
[`contribution/index.md`](../../../contribution/index.md) records the order and how
to recover a failed run, and
[`adr/0005-release-binaries.md`](../../../adr/0005-release-binaries.md) records the
decision.
