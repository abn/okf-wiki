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

A base of `/` is the whole site: the pages are written at the top of `--out`
with no wiki subdirectory, and there is no root redirect, because the wiki's own
`index.html` already occupies that path. This is the base the GitHub Action
computes for a user or organisation Pages site, so it is a normal configuration
rather than an edge case.

## Endpoints

- `/wiki/` and `/wiki/<slug>.html`: the rendered wiki, under `--base`.
- `/repo/<path>`: read-only repository mount, when `--repo` is set. Dotfiles are
  refused.
- `/healthz`: liveness probe.
- `/`: redirects to `--base`, unless `--base` is `/`, where it is the wiki
  itself.

## Shutdown

`serve` handles `SIGINT` and `SIGTERM` and drains before exiting, so a request
in flight finishes rather than being cut mid-body. That is the container's stop
path, and a truncated page body is worse than a slightly slower stop. The drain
is bounded: if a connection does not close within three seconds the server
closes it and the process exits.

A `ReadHeaderTimeout` of ten seconds, a generous `WriteTimeout` and an
`IdleTimeout` are set, so a stalled client cannot hold a connection open
indefinitely.
