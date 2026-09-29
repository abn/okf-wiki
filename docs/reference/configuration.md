---
type: Reference
title: Configuration
description: Every flag and environment variable, with defaults.
tags: [reference, configuration]
---

# Configuration

Resolution order is flag, then environment variable, then default.

| Flag | Environment | Default | Purpose |
| :--- | :--- | :--- | :--- |
| `--content` | `OKF_WIKI_CONTENT` | auto-detect | OKF bundle directory |
| `--out` | `OKF_WIKI_OUT` | `.scratch/wiki` | Rendered output root |
| `--base` | `OKF_WIKI_BASE` | `/wiki/` | URL prefix, and the path inside the output root |
| `--repo` | `OKF_WIKI_REPO` | empty | Repository root served at `/repo/` |
| `--vendor` | `OKF_WIKI_VENDOR` | `mermaid` | Directory holding the mermaid bundle |
| `--theme` | `OKF_WIKI_THEME` | empty | Theme directory layered over the embedded default |
| `--brand` | `OKF_WIKI_BRAND` | `okf-wiki` | Wordmark |
| `--brand-sub` | `OKF_WIKI_BRAND_SUB` | `docs` | Subtitle label |
| `--title` | `OKF_WIKI_TITLE` | `Wiki` | Document title suffix |
| `--sections` | `OKF_WIKI_SECTIONS` | empty | Section order, `id:Title` pairs |
| `--addr` | `OKF_WIKI_ADDR` | `0.0.0.0:8080` | Listen address (serve only) |
| `--open` | `OKF_WIKI_OPEN` | `false` | Open a browser (serve only) |

`--base` is both the URL prefix and the directory the pages are written into
under `--out`, so a rendered directory can be served by any static file server
unchanged. It must not contain `.` or `..` segments.

## Endpoints

- `/wiki/` and `/wiki/<slug>.html`: the rendered wiki, under `--base`.
- `/repo/<path>`: read-only repository mount, when `--repo` is set. Dotfiles are
  refused.
- `/healthz`: liveness probe.
- `/`: redirects to `--base`.
