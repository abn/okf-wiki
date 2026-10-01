---
type: Guide
title: GitHub Action
description: Rendering a bundle and publishing it to GitHub Pages with the okf-wiki action.
tags: [usage, github-actions, pages]
---

# GitHub Action

The `okf-wiki` action renders an OKF bundle in a workflow and, optionally,
publishes it to GitHub Pages. It runs the published image, so a consumer needs
neither a Go toolchain nor a diagram bundle build.

## Render only

```yaml
- uses: actions/checkout@v7

- uses: abn/okf-wiki/action@v1
  with:
    content: docs
```

The default: build and stop. Nothing is published and no write permission is
needed, so it is safe on pull requests from forks. Upload the result yourself if
you want an artifact:

```yaml
- uses: actions/upload-artifact@v4
  with:
    name: wiki
    path: _site
```

## Publish to GitHub Pages

```yaml
name: Publish wiki

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: false

jobs:
  publish:
    runs-on: ubuntu-24.04
    environment:
      name: github-pages
      url: ${{ steps.deploy.outputs.deploy-url }}
    steps:
      - uses: actions/checkout@v7

      - id: deploy
        uses: abn/okf-wiki/action@v1
        with:
          content: docs
          deploy: true
```

`deploy: true` uploads the output as a Pages artifact and hands it to
`actions/deploy-pages`. It needs `pages: write` and `id-token: write`, and Pages
set to build from a workflow.

To publish from a branch instead, render with `deploy: false` and push `_site`
yourself.

## Inputs

| Input | Default | Purpose |
| :--- | :--- | :--- |
| `content` | `docs` | bundle directory, relative to the workspace |
| `out` | `_site` | build output directory |
| `theme` | empty | theme directory in the repository |
| `theme-ref` | empty | shared theme as `owner/repo@ref` |
| `image` | `ghcr.io/abn/okf-wiki:latest` | image to render with |
| `base` | `auto` | URL prefix the site is served under |
| `brand` | `okf-wiki` | wordmark |
| `brand-sub` | `docs` | subtitle label |
| `version-tag` | empty | freeform version label beside the wordmark |
| `title` | `Wiki` | document title suffix |
| `sections` | empty | section order, `id:Title` pairs |
| `deploy` | `false` | publish to GitHub Pages |

All but `base` and `deploy` are flags on `okf-wiki render`, passed through as
given. The environment equivalents work too, so `OKF_WIKI_THEME` in the job
environment is picked up. See
[`reference/configuration.md`](../reference/configuration.md).

Outputs are `site-dir`, `base`, and `deploy-url`.

## The base path

`base` defaults to `auto`, which reads the repository name: a project Pages site
is rendered under `/<repo>` and a user or org site at the domain root.

The renderer puts the site at the base inside `out`, so the site lands in
`_site/<repo>/`. That is the directory to deploy: Pages serves the artifact root at
the project path, so the artifact has to be the wiki itself, and uploading the
outer `_site` would serve the renderer's root redirect at the site root, where
it points at itself. `site-dir` reports the directory to deploy, and it is what
the action uploads.

Pass an explicit value to override the guess:

```yaml
with:
  base: /            # a user or org site, or a custom domain
  base: /handbook    # a project site under a different path
```

## Themes

`theme` is a path to a theme directory in the repository, laid over the embedded
default. It must hold a `theme.json`; the renderer rejects a malformed manifest,
an unknown mode, or a `replace` naming a file that is not there.

```yaml
with:
  theme: themes/acme
```

`theme-ref` pulls a theme from another repository, for one theme shared across
wikis. It takes precedence over `theme`.

```yaml
with:
  theme-ref: abn/okf-theme@v1
```

See [`reference/themes.md`](../reference/themes.md) for the manifest contract.

## Runtime

Podman, preinstalled on the GitHub-hosted Linux runners, so `ubuntu-latest` needs
no setup. Self-hosted runners need podman installed. Windows and macOS are not
supported.

## See also

- [`usage/cli.md`](cli.md) - the same flags from a terminal
- [`usage/container.md`](container.md) - running the image by hand
- [`reference/themes.md`](../reference/themes.md) - the theme manifest
