---
type: Log
title: Knowledge base change log
description: Evolution of this OKF documentation bundle only, not software releases.
tags: [log]
---

# Change log

This file tracks changes to the documentation bundle itself: pages added,
deprecated, or restructured. It is deliberately separate from software releases.

## Test bed section

- A `testbed/` section that exercises the whole rendering surface on viewable
  pages: callouts at all five levels, tables and task lists and footnotes, code
  across lexers and one language with none, four mermaid shapes including an
  invalid diagram, and heading ids that collide on purpose.
- It exists so a theme author can see every construct at once, and so a change to
  the renderer has a visible regression target rather than only a unit test.
- `testbed/headings.md` currently renders a duplicated `id="setup-2"`. That is
  the bug, left visible on purpose until the anchor fix lands.

## Initial bundle

- Pages: overview, design, usage, reference and contribution, covering the
  renderer, the command line, the container, the GitHub Action, the theme
  contract and the configuration reference.- Three architecture decision records: a standalone Go renderer rather than the
  TypeScript package; the action shipping a container reference rather than a
  release binary; and themes as runtime directories rather than a compiled-in
  skin.
- Themes are a first-class concept. A theme is a directory layered over an
  embedded default at run time, so re-skinning a deployment is a read-only mount
  and an environment variable rather than a new image. `reference/themes.md`
  documents the manifest, the resolution order, the DOM contract and the asset
  contract; `design/branding.md` covers what a theme owns.
- `--base` is both the URL prefix and the path inside the output directory, so a
  rendered tree serves from any static host unchanged.
- The default theme, `paper`, bundles Figtree, Inter and JetBrains Mono, recorded
  in `THIRD-PARTY.md` alongside the offline diagram runtime licenses.

## Test bed

- Added a `testbed` section: callouts, blocks, code, diagrams and headings. It
  exists so a theme author can see the whole rendering surface in one place, and
  so a rendering change has a visible regression target rather than only a unit
  test.
- `testbed/headings.md` deliberately repeats headings and mixes numbered
  headings with plain ones, so an anchor collision stays visible on a page in
  this bundle rather than only in a fixture. Its ids are now unique.
- `testbed/blocks.md` documents the three link kinds but carries no live link
  for the third. The escaping form only resolves with `--repo`, which this
  bundle is published without.
- `testbed/nested/setup/first-run.md` is a page two directories deep, so nested
  content and links that climb back out of it are visible in the bundle.
