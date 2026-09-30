---
type: Decision
title: The action ships a container reference, not a binary
description: Why the GitHub Action renders through the published image instead of downloading a release binary.
tags: [adr, decision, distribution]
status: stable
---

# 0002 The action ships a container reference, not a binary

## Context

The action has to run a render in a consumer's workflow. Either download a
release binary or run the published image.

This project publishes no releases. `publish.yml` pushes to ghcr on every push to
`main` and on `v*` tags, and that is the only channel. A binary would need a
release pipeline first, and would have to carry the mermaid bundle too, since
that is built by a Node stage in the image.

## Decision

The action runs `ghcr.io/abn/okf-wiki:latest` with podman, overridable through
the `image` input and the `IMAGE_NAME` repository variable. No release artifacts.

## Rationale

- The image already carries the pinned mermaid and ELK bundle, so a Pages build
  gets offline diagrams with no CDN call and no npm install.
- Nothing to keep in step with tags. Image and action move together on `main`.
- The action cannot drift from the supported runtime.
- Podman, not Docker. Both are preinstalled on the runners and the rest of this
  project is podman-based.

## Consequences

- Podman is required. Preinstalled on the GitHub-hosted Linux runners; Windows
  and macOS are unsupported.
- The image runs as uid 10001 and cannot write the runner-owned workspace.
  Rootless podman maps container root to the runner user, so the render runs with
  `--user 0:0` rather than reconciling ids.
- The image is a moving tag. A consumer can pin `image` to a release tag.
