---
type: Decision
title: Releases carry binary archives
description: Why each release attaches cross-compiled Go archives, and why the action still runs the container image.
tags: [adr, decision, distribution]
status: stable
---

# 0005 Releases carry binary archives

## Context

The container needs podman, and `go install` needs a Go toolchain. Neither gives
someone a single file to download when they want one render on a laptop, or a
known build they did not compile. ADR 0002 chose the container for the action.
ADR 0003 rejected goreleaser for images and releases, and called binary archives
worth revisiting if they were ever wanted.

## Decision

Every release attaches static archives built from its tag:

- linux, macOS and Windows on amd64 and arm64, six targets in all;
- one Linux runner cross-compiles them with `CGO_ENABLED=0 go build`, in a
  reusable workflow that takes the tag as an input;
- each archive holds the binary, `LICENSE`, `README.md`, `THIRD-PARTY.md` and the
  mermaid runtime under `mermaid/`, and `checksums.txt` covers the set;
- the native archive is run before upload and must report the tag's version.

The action still runs the container image, and the image keeps its own publish
path. The archives are the only new artifact.

## Rationale

- The binary is pure Go, so one runner cross-compiles all six targets in
  seconds. macOS runners would add cost without adding capability.
- The archive carries the diagram runtime because the binary alone cannot render
  a bundle with a diagram without a CDN call (0002). It is the runtime the image
  already builds, from the same Node sources.
- `go build` is the toolchain the repository already uses. goreleaser stays
  rejected, since it would add a second configuration language for a loop over
  `GOOS` and `GOARCH`.
- Archives, not installers. No brew tap, RPM or deb to keep in step, and no
  package index to host.
- Stable asset names mean `releases/latest/download/<name>` resolves without an
  API call.

## Consequences

- Adding a target is one line in `binaries.yml`.
- The release job runs Node as well as Go, since the diagram runtime is built
  with the same `npm ci` and esbuild commands as the image.
- A tag whose binary does not report the tag's version fails the release job
  instead of publishing a mislabelled archive.
- Archive timestamps are not normalized, so a rebuild produces different bytes
  and the checksums identify a build rather than prove it can be reproduced.
- A release that predates the workflow is filled in by dispatching it with the
  tag.
