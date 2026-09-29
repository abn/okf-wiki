---
type: Overview
title: What okf-wiki is
description: Purpose, scope, and the problems okf-wiki solves.
tags: [overview]
---

# What okf-wiki is

okf-wiki turns a directory of Markdown with YAML frontmatter (an OKF v0.2
bundle) into a small static site: sidebar navigation, an on-page table of
contents, syntax-highlighted code, GitHub alert callouts, Mermaid diagrams, and
a full-text search modal. It then serves that site.

It exists so a project can keep plain Markdown as the source of truth and still
browse it as a wiki, without a build pipeline per project.

## Scope

In scope: rendering OKF frontmatter and links, client-side search, diagram
rendering, theming, and a container that serves a mounted bundle.

Out of scope: editing content through the wiki, authentication, multi-user
state, and any hosted deployment. It is a read-only reader.

## Two ways to run

- As a binary: `okf-wiki serve --content ./docs`.
- As a container: `podman run --rm -v $(pwd)/docs <image>`, where the content
  directory is discovered from the mount. See [Usage](usage/index.md).

## Theming

The look of a wiki is a theme, and a theme is a directory rather than a build
step. One is embedded in the binary, and `--theme DIR` layers a directory over
it, so re-skinning a deployment is a read-only mount and an environment variable
rather than a new image. A theme restates only what it changes, which means a
recolour is a single stylesheet.

See [Themes](reference/themes.md) for the manifest and the asset contract, and
[Branding and theming](design/branding.md) for what a theme does and does not
own.
