# Change log

This file tracks changes to the documentation bundle itself: pages added,
deprecated, or restructured. It is deliberately separate from software releases.

## 2026-10-03

* **Update**: The sidebar follows one level of the folder structure: pages that
  share a sub-directory under their section are grouped under a nested
  disclosure, named from the sub-directory or its `index.md`. `design/rendering.md`
  and `reference/themes.md` say so.
* **Fix**: A linked bundle's non-Markdown files reach the output. Both walks
  now resolve their root, since `WalkDir` treats a symlinked starting point as a
  file, so a wiki assembled by linking a checkout in served its pages with every
  image and download broken. `usage/multi-site.md` says so.
* **Update**: A multi-site group index groups its entries: the wikis lead as
  cards and the folders follow under a "Folders" heading as compact rows.
  `usage/multi-site.md` and `reference/themes.md` say so.
* **Update**: `serve --watch` takes an interval, `--watch-interval` (or
  `OKF_WIKI_WATCH_INTERVAL`, default `1s`), and renders into a directory built
  beside the output that is swapped into place, so a page loaded during a
  re-render is never missing or half-written. `usage/cli.md`,
  `reference/configuration.md` and `adr/0004-watch-rerenders-the-whole-tree.md`
  carry it.
* **Creation**: Added `adr/0004-watch-rerenders-the-whole-tree.md`, which
  records that `serve --watch` re-renders every wiki on any change rather than
  the page that moved, the measured cost at 1, 5 and 20 wikis, and the decision
  to defer incremental rendering until a tree is big enough to feel it.
  `usage/cli.md` and `adr/index.md` link to it.
* **Update**: A multi-site group card is clickable across its whole face: the
  link stretches over the card, so a tap anywhere opens it while its text stays
  the title and badge. `reference/themes.md` says so.
* **Update**: `--multi-site` follows a directory symlink, including a content
  directory that is itself a symlink, and `serve --watch` sees a change inside a
  linked bundle, so a tree can be assembled by linking bundles in from their
  checkouts. `usage/multi-site.md` says so, including that a directory already
  walked is not walked again and that a container render needs the link targets
  mounted at the same absolute paths.
* **Creation**: Added `usage/multi-site.md`, a guide to publishing a directory of
  wikis as one site: what makes a directory a wiki, the generated group indexes,
  the metadata that names them, the search index, and how watching fits.
* **Update**: `usage/cli.md`, `reference/configuration.md`, `design/rendering.md`
  and `reference/themes.md` document `--multi-site` (or `OKF_WIKI_MULTI_SITE`),
  which reads the content directory as a tree: each directory with an `index.md`
  is a wiki rendered at its own path, and every other directory is a group given
  a generated index of its children.
* **Update**: `usage/cli.md` and `reference/configuration.md` document
  `serve --watch`, and `OKF_WIKI_WATCH`, which re-render the site when the bundle
  or a layered theme changes. An edit is live on the next page load, and a
  re-render that fails keeps the last good one serving.

## 2026-10-02

* **Update**: `design/rendering.md` and `reference/themes.md` describe the
  diagram card's own zoom. A diagram now opens fitted to its card and to the
  full-screen viewer rather than at natural size with its edges cut off, and the
  viewer's clone lays out at the card's text size, which stops mermaid's labels
  outgrowing the boxes they were measured for.
* **Update**: Each page now ships its Markdown source beside its HTML and offers
  it from a Download Markdown control under the title, and printing produces a
  single-column document with the controls hidden, headings kept with their
  content, tables broken with a repeated header, and diagrams fitted to the
  page. `design/rendering.md` describes both, and `reference/themes.md` and
  `usage/cli.md` list the class and the output.
* **Update**: `design/rendering.md` documents both in-bundle link forms, relative
  to the page and from the bundle root, and `testbed/blocks.md` carries a
  bundle-relative link. A root-relative link now resolves from the bundle root
  rather than from the page, so it survives the page moving.

## 2026-10-01

* **Update**: A `--nav-links` flag (and `OKF_WIKI_NAV_LINKS`), plus a `nav-links`
  action input, adds links to the header as `Label=URL` pairs. An external target
  opens in a new tab and is marked with an arrow; an in-bundle path resolves as a
  body link and opens in the same tab. On a phone they move into the drawer, the
  in-wiki links first under "On this wiki" and the rest under "Elsewhere".
* **Update**: A `--version-tag` flag (and `OKF_WIKI_VERSION_TAG`), plus a
  `version-tag` action input, renders a freeform label as a pill beside the
  wordmark when set and nothing when unset. The flag reference, the branding
  page and the action inputs document it.
* **Update**: `design/rendering.md` documents the `sources` construct: how a
  footnote becomes a numbered citation to the generated Sources section, how an
  entry resolves, and which credibility signals it shows.

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
