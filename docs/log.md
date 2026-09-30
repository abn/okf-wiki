# Change log

This file tracks changes to the documentation bundle itself: pages added,
deprecated, or restructured. It is deliberately separate from software releases.

## 2026-09-30

* **Update**: Tag chips on every page now link to a tag page listing all pages
  carrying the tag, and `#tag` in the search modal searches tags rather than
  page text.
* **Update**: `testbed/blocks.md` now carries a `sources` list, so numbered
  citations, the generated Sources section and the scope-descriptor case are
  visible on a page in this bundle rather than only in a fixture.
* **Update**: Section headings are one step smaller with more air above, carry
  a `#` permalink on hover, and the table of contents lists level two only.
* **Update**: Diagrams render in cards with a type label, copy and expand
  affordances, at one size and natural width, and a diagram that will not
  parse shows its source and the parser message instead of a blank block.
* **Creation**: Added a `testbed/` section that exercises the whole rendering
  surface on viewable pages: [callouts](testbed/callouts.md) at all five
  levels, [blocks](testbed/blocks.md) with tables, task lists and footnotes,
  [code](testbed/code.md) across lexers and one language with none, four
  [diagrams](testbed/diagrams.md) including an invalid one, and
  [headings](testbed/headings.md) that repeat and collide on purpose. It exists
  so a theme author can see every construct at once, and so a rendering change
  has a visible regression target rather than only a unit test.
* **Creation**: Added [first-run](testbed/nested/setup/first-run.md), a page
  two directories deep, so nested content and links that climb back out of it
  are visible in the bundle.
* **Creation**: Added an `über` section, whose directory name begins with a
  multi-byte character. Section titles are built from directory names, and a
  non-ASCII name used to produce a replacement character.
* **Update**: `testbed/headings.md` deliberately repeats headings and mixes
  numbered headings with plain ones, so an anchor collision stays visible on a
  page in this bundle rather than only in a fixture. Heading ids are unique.
* **Update**: `testbed/blocks.md` documents the three link kinds but carries
  no live link for the third. The escaping form only resolves with `--repo`,
  which this bundle is published without.

## 2026-09-29

* **Creation**: Established the bundle: overview, design, usage, reference
  and contribution, covering the renderer, the command line, the container,
  the GitHub Action, the theme contract and the configuration reference.
* **Creation**: Recorded three architecture decision records: a standalone Go
  renderer rather than the TypeScript package; the action shipping a container
  reference rather than a release binary; and themes as runtime directories
  rather than a compiled-in skin.
* **Creation**: Documented themes as a first-class concept. A theme is a
  directory layered over an embedded default at run time, so re-skinning a
  deployment is a read-only mount and an environment variable rather than a
  new image. `reference/themes.md` documents the manifest, the resolution
  order, the DOM contract and the asset contract; `design/branding.md` covers
  what a theme owns.
* **Creation**: Documented that `--base` is both the URL prefix and the path
  inside the output directory, so a rendered tree serves from any static host
  unchanged.
* **Creation**: Recorded that the default theme, `paper`, bundles Figtree,
  Inter and JetBrains Mono, alongside the offline diagram runtime licenses.
