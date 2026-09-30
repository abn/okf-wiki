---
type: Decision
title: A standalone Go renderer, not the TypeScript package
description: Why okf-wiki is a self-contained Go binary rather than a reuse of the abn.is wiki-render package.
tags: [adr, decision]
status: stable
---

# 0001 Standalone Go renderer

## Context

The abn.is site already ships an OKF renderer: a TypeScript package
(`packages/wiki-render`) with a shared render spine, used by Astro routes. A
second renderer for the homelab wiki was written in Go (goldmark plus Chroma).

Both approaches render the same bundle format.

## Decision

Extract the Go renderer into a standalone project and ship it as a static binary
and a container. Do not build on the TypeScript package for this tool.

## Rationale

- A single static binary needs no Node runtime, bundler, or `node_modules` in
  the container. The image is built from a multi-stage Go build and a small
  runtime layer.
- The Go renderer has four dependencies, all pure Go, and no SSR framework. It
  starts in milliseconds and renders a bundle in one pass.
- The TypeScript package is coupled to the Astro build (Vite, SSR, Cloudflare
  Workers) and is not published, so consuming it from a container would drag in
  that toolchain.

## Consequences

- Two renderers exist and must stay aligned on OKF parsing. The Go one is the
  reference for this tool; the shared format is the contract.
- Feature parity with the Astro renderer is not a goal. This tool targets
  reading and search, not the full site.
