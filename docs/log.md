# Change log

This file tracks changes to the documentation bundle itself: pages added,
deprecated, or restructured. It is deliberately separate from software releases.

## 2026-10-14

* **Feature**: Code and text blocks carry a copy button in their top-right
  corner, added client-side and a11y-compliant (a real button with an
  accessible name and focus ring). It uses the same copy glyph as the diagram
  toolbar and is hidden when printing. `design/rendering.md` and
  `reference/themes.md` say so.

* **Feature**: A full width reading mode, toggled by an icon beside the theme
  toggle, lets the article take the width the sidebars held. The rails collapse
  into the breadcrumb bar's drawer and On this page control, and the choice is
  persisted across page loads. `reference/themes.md` says so.
* **Fix**: A sidebar group heading no longer sits a step deeper than the loose
  pages and leaf groups beside it. The group's `summary` carries the row's
  horizontal padding, and the link inside it also picked up the generic
  `.side li a` padding, so a second-level group such as a section's
  sub-directory read as a third-level subitem of the page above it. The inner
  link now drops that padding, so the group stays aligned with its siblings
  while its own pages still hang off the hairline rule.
  `reference/themes.md` says so.
* **Update**: A `lang` action input, matching the `--lang` flag and
  `OKF_WIKI_LANG`, sets the language the rendered pages declare, defaulting to
  `en`; `action.yml` and the action inputs document it.
* **Feature**: Images in a page's prose open in the same viewer as diagrams, with pan and zoom. Clicking one enlarges it and the viewer scales it to fit; the brand mark and other chrome stay out of it. The `wiki-lbx-pill` now reads Image rather than Diagram there, and the docs record it under `reference/themes.md`.

## 2026-10-13

* **Fix**: An alert marker after an inline node no longer becomes a callout. The
  probe that finds the marker read only text nodes, so a marker following a
  link, an image, a code span or emphasis looked like it opened the line: the
  callout card appeared with the marker stripped and only the inline node left
  as its body. The probe now stops at the first non-text node, and the space
  after the marker is skipped within its own node, so a space belonging to a
  later node survives. `design/rendering.md` says so, and `testbed/callouts.md`
  shows the markers that now stay blockquotes.
* **Fix**: The search dialog keeps focus, names its input, and says what it
  found. The dialog declared `aria-modal`, so Tab was expected to stay inside
  it, and a walk of Tab presses on a freshly opened dialog left the results and
  reached the page chrome behind. The candidates are collected on each keypress,
  because the rows are rebuilt on every keystroke. The input was named by its
  placeholder, which is a hint rather than a name and disappears as soon as a
  reader types. Nothing was announced at all, so the result count, and in a tree
  the wikis it came from, now go to a polite status node beside the rows,
  clipped out of the layout rather than hidden. `reference/themes.md` says so.
* **Feature**: A wiki declares the language its pages are written in. `<html
  lang="en">` was hardcoded, so a screen reader and a read-aloud voice picked
  English whatever language the bundle was written in. A `--lang` flag and its
  `OKF_WIKI_LANG` environment variable set it, defaulting to `en`, and the root
  redirect carries the same language as the pages it redirects to. A value that
  is not a language tag is refused, since a browser ignores a tag it cannot read
  and the page would then read as English whatever was asked for.
  `reference/configuration.md` says so.
* **Fix**: A callout is a note rather than a landmark, and a table header says
  which direction it heads. A callout carried `role="region"`, so every card on a
  page added another entry to the landmark list, and its `aria-label` repeated
  the word the visible title in `.callout-title` already spells, which a screen
  reader announced twice. The role is now `note`, which is not a landmark, and
  the visible title is what names it. A table header was a bare `<th>`, so a
  screen reader read a cell as "Left Useful information" with nothing saying
  that Left is the column it sits under, and every header cell now carries
  `scope="col"`. The alignment a table declares is unchanged and a body cell
  gains no scope, since goldmark marks no cell as a row header.
  `reference/themes.md` says so.
* **Feature**: The sidebar's active link carries `aria-current` beside the active
  class, and the labels the default theme drew in uppercase text are small caps
  now. The class only painted the position, so a screen reader had nothing to
  announce for the page a reader was on. An uppercase text transform reaches a
  screen reader as uppercase text, so a label such as "Previous" was announced
  shouted, while small caps keep the same weight and leave the text as it was
  written. `reference/themes.md` says so.
* **Feature**: A rendered diagram has an accessible name and is announced as one
  image. The SVG had nothing for a screen reader to announce, and a reader view
  read the drawing as a run of node labels. The name comes from the fence's own
  `accTitle` and `accDescr` when the author wrote them, and from the nearest
  preceding heading otherwise, the type word is the readable one rather than
  mermaid's internal name, and the drawing is one image rather than every shape
  in it. `design/rendering.md` says so.

## 2026-10-12

* **Fix**: An alert marker in lower or mixed case renders as its callout. The
  marker was matched as written, so `> [!warning]` fell through to a plain
  blockquote with the marker visible as text. GitHub matches it without regard
  to case, and the renderer now does too, so every spelling of the five kinds
  gives the same card. A marker outside those five is still left alone.
  `design/rendering.md` says so, and `testbed/callouts.md` shows the lower and
  mixed case forms.

## 2026-10-11

* **Fix**: The search modal no longer hangs when a term is typed before the
  index has loaded. A caller that asked while the first fetch was in flight was
  resolved with a not-yet-loaded index, so `runSearch` called `fetchIndex` again
  from its own `.then`; both ran as microtasks, so the chain never yielded to the
  event loop and the request it waited on could never settle. The in-flight
  request is now shared, and a failed load leaves an empty catalogue rather than
  a null one. The same guard covers the `#tag` catalogue.

## 2026-10-07

* **Update**: The sidebar's third level reads as a group's leaves. A page whose
  title repeats its group heading drops the shared prefix, so the row fits one
  line at any rail width, the pages hang off a hairline rule rather than a
  deeper indent, and a section opens one group at a time so a rail of many
  groups stays short. A page can override its rail label with `nav` in
  frontmatter. `design/rendering.md` and `reference/themes.md` say so.
* **Update**: The command line page's tree sample uses a generic `example` group
  name, so no real client name appears in the published bundle.
* **Page**: A multi-site tour shows what a tree of wikis looks like, with
  screenshots of a group index, search grouped across the tree, the header's
  breadcrumb trail, and the card layout on a phone. `usage/index.md` links it.
* **Feature**: A multi-site tree has one brand. The brand anchor targets the
  tree index through a new `{{.HomeHref}}`, which is `{{.Base}}` for a single
  bundle and the tree base for a tree page, so the mark and the wordmark mean
  the deployment home from any depth. The breadcrumb becomes a trail from the
  tree root down to the current wiki and page
  (`Wiki / Clients / Example / Operations / Guides / Setup`), with each folder
  linking to its group index and the wiki title linking to the wiki home; a
  group index ends at the group name and the tree root shows the title once. A
  tag page in a tree keeps that trail and then reads `Tags / #tag`, since it is
  a page of its own wiki. A single bundle's header and breadcrumb are byte for
  byte unchanged.
  `usage/multi-site.md`, `design/branding.md` and `reference/themes.md` say so.
* **Fix**: A multi-site group index's wiki cards line up when they sit side by
  side. From the 568px width at which the card grid shows two columns, every card
  reserves two lines for its title and three for its description, so a short card's
  text sits on the same lines as a taller neighbour's, every row is one height,
  and the wiki badge sits on the title's first line rather than the middle of a
  wrapped title. Below that the grid is one column, so each card sizes to its
  content: no reserved line under a one-line title and no clamp cutting a long
  title or description short.
* **Feature**: Search spans a multi-site tree. The tree writes one aggregate
  `search-index.json` at its base holding the entries of every wiki that
  rendered, each labelled with the wiki it came from, and a page fetches it
  through a new `{{.SearchBase}}` / `window.__WIKI_SEARCH`. The client groups
  results under a heading per wiki and adds an All wikis / This wiki toggle.
  A single bundle's index and the `#tag` path are unchanged.
  `usage/multi-site.md`, `design/rendering.md`, `design/branding.md` and
  `reference/themes.md` say so.
* **Fix**: A group index of a tree published under a non-root base no longer
  points at theme assets and an empty search catalogue written elsewhere. A group
  index renders with the tree's own base, so its assets and catalogue now sit
  under the base directory beside it instead of at the output root, which the two
  paths share only when the base is `/`. `usage/multi-site.md` says so.
* **Fix**: A multi-site content root that is itself a wiki keeps its own
  `search-index.json` at the base instead of having it replaced by the labelled
  aggregate, so its search reads as a single bundle's with no per-wiki headings.
  `design/rendering.md` and `usage/multi-site.md` say so.
* **Fix**: The search modal's footer wraps rather than scrolling horizontally on
  a narrow viewport, and the All wikis / This wiki toggle stays on one line.
* **Docs**: The tree aggregate is reachable from a group index, so a search there
  finds hits anywhere in the tree rather than only the wikis below it. Each
  wiki's own index and tag catalogue sit beside the tree aggregate.
  `usage/multi-site.md` and `adr/0004-watch-rerenders-the-whole-tree.md` say so.
* **Fix**: The search modal fits a phone viewport. The search input can shrink
  below its intrinsic width, so the header's close control stays inside the
  modal at widths under 390px.

## 2026-10-03

## 2026-10-04

* **Fix**: A wiki of a tree that cannot be read is skipped and reported instead
  of failing the whole render, so one stale link in a linked checkout no longer
  takes every wiki offline. The skipped wiki leaves no output and no catalog
  card. `design/rendering.md` says so.
* **Fix**: The wordmark's mark is preloaded from the head at high priority and
  requested at the same versioned URL, so it no longer paints a step behind the
  rest of the header. `design/rendering.md` says so.
* **Fix**: Every response carries an `ETag` from the file's bytes beside
  `Last-Modified`, so a re-render, which moves every file's modification time,
  no longer turns an unchanged asset's next request into a full `200`.
  `design/rendering.md` says so.
* **Fix**: A dark page no longer flashes white between pages. The ground is
  painted on `html` as well as `body`, since the canvas a browser shows during a
  cross-document navigation is the root background, and the pre-paint restore
  sets `color-scheme` so the browser's default canvas is dark before the
  stylesheet applies. `reference/themes.md` says so.
* **Fix**: The theme's three stylesheets are linked from the head rather than
  `@import`-ed into one, so they load in parallel instead of as a serial chain,
  and an `index.html` request is served in place rather than answered with a 301
  to `./`, so an index link does not cost a redirect before the document.
  `design/rendering.md` says so.
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
