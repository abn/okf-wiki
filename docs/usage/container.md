---
type: Guide
title: Container
description: Serving a mounted OKF bundle with the okf-wiki container image.
tags: [usage, container, podman]
---

# Container

The image serves any OKF bundle mounted from the host. The content directory is
discovered from the mount, so no container path is required.

```bash
# Mount at the same path; okf-wiki finds it via the bind mount.
# The :Z relabels the mount so an SELinux host lets the container read it.
podman run --rm --network=host -v "$(pwd)/docs:ro,Z" ghcr.io/abn/okf-wiki:latest

# Or mount explicitly at /content (auto-detected) and publish a port.
podman run --rm -p 8080:8080 -v "$(pwd)/docs:/content:ro,Z" ghcr.io/abn/okf-wiki:latest
```

Then open `http://127.0.0.1:8080/wiki/`. The first form uses host networking, so
the wiki is reachable on the host's `:8080` without publishing a port.

## Configuration

Override any setting with environment variables:

```bash
podman run --rm -p 8080:8080 \
  -v "$(pwd)/docs:/content:ro" \
  -e OKF_WIKI_BRAND="acme." \
  -e OKF_WIKI_BRAND_SUB=docs \
  -e OKF_WIKI_TITLE="Acme Docs" \
  ghcr.io/abn/okf-wiki:latest
```

## Mounting a theme

A theme is a read-only bind mount and one environment variable. No rebuild, and
no new image, which is the whole reason the theme is a runtime addition rather
than part of the binary.

```bash
podman run --rm -p 8080:8080 \
  -v "$(pwd)/docs:/content:ro" \
  -v "$(pwd)/themes/acme:/theme:ro,Z" \
  -e OKF_WIKI_THEME=/theme \
  ghcr.io/abn/okf-wiki:latest
```

The mounted directory is layered over the theme embedded in the image, so it only
has to carry what it changes. A theme that replaces one stylesheet is a single
file; see [`reference/themes.md`](../reference/themes.md).

Compose expresses the same thing, commented out so the default path needs no
theme at all:

```yaml
    volumes:
      - ./docs:/content:ro
      - ./themes/acme:/theme:ro
    environment:
      OKF_WIKI_THEME: /theme
```

## Offline diagrams

The image bakes a pinned mermaid and ELK bundle, so Mermaid diagrams render with
no network access. Running the binary without a vendor directory falls back to
the mermaid CDN.

## Building

```bash
make container/build
make container/run
```
