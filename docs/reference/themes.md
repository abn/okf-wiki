---
type: Reference
title: Themes
description: The theme manifest, how a theme is layered over the embedded default, and the asset and template contracts.
tags: [reference, theme, manifest]
---

# Themes

A theme is a directory. okf-wiki embeds one, and `--theme DIR` layers a
directory over it. Resolution happens once, before rendering, so a render never
reads a theme path and never resolves outside the theme directory.

## The manifest

A theme directory must contain a `theme.json`. Every field is optional: a
manifest with an empty `assets` map inherits the default wholesale, which is the
smallest useful theme there is.

```json
{
  "name": "acme",
  "version": "1.2.0",
  "description": "Acme house style.",
  "template": "template.html",
  "slots": { "head": "head.html", "bodyEnd": "body-end.html" },
  "assets": {
    "tokens.css": { "mode": "replace" },
    "fonts.css": { "mode": "inherit" },
    "wiki.css": { "mode": "replace" },
    "fonts/": { "mode": "replace" },
    "brand-mark.svg": { "mode": "replace" },
    "favicon.svg": { "mode": "inherit" },
    "search.js": { "mode": "inherit" },
    "diagrams.js": { "mode": "drop" }
  }
}
```

### Fields

| Field | Meaning |
| :--- | :--- |
| `name` `version` | the theme's identity. An override that omits them keeps the default's. |
| `description` | one line, recorded in the output's `theme.json`. |
| `template` | path to the shell template, relative to the theme root. |
| `slots` | named fragments, reachable from the template as `{{.Slot "name"}}`. |
| `assets` | per-asset resolution, keyed by output path. |

### Asset modes

| Mode | Effect |
| :--- | :--- |
| `inherit` | take the default's file. This is the default when `mode` is omitted, and the behaviour for any key the manifest does not mention. |
| `replace` | use this theme's file or subtree. The file lives at `assets/<key>` in the theme directory. |
| `drop` | emit no file for that key. |

A key ending in `/` is a subtree. `"fonts/"` with `replace` swaps the whole font
directory in one line, and no file from the default's `fonts/` survives it,
which is what makes a face swap work without restating ten filenames.

## Resolution order

1. The embedded default is read as the base layer.
2. The override's manifest is read. Each `drop` and `replace` is applied over the
   base, in the order the keys are sorted.
3. The template and slots are taken from the topmost layer that declares them.
4. The resolved bytes are fingerprinted into the `?v=` asset version, so a theme
   edit always busts the browser cache. This is why the version differs between
   two themes even when they share a stylesheet.

A malformed manifest is a configuration error and fails the render rather than
shipping a broken page. So is a `replace` naming a file that is not in the
directory, a `mode` that is not one of the three above, and a template that does
not parse.

A template that parses but then fails against the page data is the same class of
error and fails the render too. Referring to a field that does not exist, or to a
method that panics on what it is given, is a mistake in the template rather than
in the content, and the error names the page and the theme so it can be found.
Parsing alone is not enough to prove a template works: a template is only fully
exercised when a page is rendered through it, which is what the render does.

## The asset contract

These are the keys the default theme publishes, and the ones worth replacing:

| Key | Role |
| :--- | :--- |
| `tokens.css` | design tokens. The cheapest thing to override, and usually the only thing a recolour needs. |
| `wiki.css` | layout and prose, bound to the class-name contract. |
| `fonts.css` | `@font-face` declarations, pointing at `fonts/`. |
| `fonts/` | the font files themselves. |
| `brand-mark.svg` `favicon.svg` | the header mark and the browser icon. |
| `search.js` | the search client. Theme-agnostic; the default's works unchanged. |
| `diagrams.js` | the diagram runtime. The default's reads its palette from `--diagram-*` custom properties, so a theme that inherits it recolours every diagram by declaring those and nothing else. The default declares a light set in `:root` and redeclares the whole set under both dark grounds, since one value cannot serve both; a theme that declares them once gets one palette in both. |

A theme may add keys the default does not have. A key only the default declares
cannot be removed except by dropping it.

## The DOM contract

The default theme's `template.html` is the reference. If a theme keeps it and
only restyles, this section does not apply. If a theme replaces it, the renderer
still guarantees some HTML and the two clients still look for specific ids, so
these are hard requirements rather than style.

### Ids the clients require

| Id | Needed by | If it is missing |
| :--- | :--- | :--- |
| `searchBtn` | `search.js` | the script throws on its first `addEventListener`, so search never initialises |
| `searchInput` `searchResults` `searchBackdrop` | `search.js` | search returns early and does nothing |
| `searchCloseBtn` | `search.js` | the modal opens and cannot be dismissed |
| `menuBtn` `drawerBackdrop` `sideDrawer` | the drawer script in the template | the drawer script throws, so the mobile menu and Escape-to-close both die |
| `themeToggle` | the toggle script in the template | the toggle does not respond |
| `main` | the skip link's `href="#main"` | the skip link jumps nowhere |

The three behaviours above, the drawer, the theme toggle and the pre-paint
restore that avoids a flash of the wrong ground, are **inline in the template**,
not in a client file. A theme that replaces `template.html` takes on all three.

### Classes the renderer writes

Go emits these, so a theme's stylesheet is the only thing that styles them:

- `nav.side`, with `details.side-sec`, `summary.side-summary`, a
  `side-title-link` or `side-title-text`, and `side-chevron`. A section with no
  child pages is emitted as `side-leaf` rather than a `details`, so it carries no
  disclosure triangle.
- `.active`, on the current page's sidebar link and on the current section's
  title, which is how a theme marks position
- `.breadcrumb`, `.crumb-sep`, `.crumb-cur`
- `.prose`, `.page-lead`, `.page-meta`, `.chip`, `.chip-type`, `.tag-link`, and
  `.source-ref` on a citation that resolves to a sources entry
- Tag pages: `.kicker`, `.tag-hash`, `.tag-often`, `.tag-all-link`,
  `.tag-controls`, `.seg` with `aria-pressed` buttons, `.tag-check`,
  `.tag-group`, `.tag-row` with `.tag-row-head`, `.tag-badges` and
  `.tag-row-meta`, `.tag-muted`, `.tag-all`, and `a.chip[aria-current]` for
  the current tag. The grouping and deprecated toggles, and whole-row
  navigation, are driven by an inline script on `data-tag-list`,
  `data-tag-group`, `data-tag-row`, `data-tag-setmode`, `data-tag-dep` and
  `data-tag-dep-note` hooks, so they work without new assets.
- Content layout: `.toc-aside` with `.toc-aside-title` and `.toc-list`,
  `.toc-btn` with `.toc-panel`, and `.prevnext` with `.prevnext-card`,
  `.prevnext-prev`, `.prevnext-next`, `.prevnext-dir` and `.prevnext-title`.
  The header's `{{.TopNav}}` links sit in `.top-nav`, each external one carrying
  `.nav-ext` on its arrow. The drawer repeats them under `.side-links`: the
  in-wiki `{{.TopNavInternal}}` first, then `{{.TopNavExternal}}`, each under a
  `.side-links-title`.
- `.toc`, `.heading-anchor`, and `.callout` with
  `.callout-title`, `.callout-icon`, `.callout-body` and the per-kind modifiers
  `callout-note`, `callout-tip`, `callout-important`, `callout-warning`,
  `callout-caution`
- `pre.mermaid`, `pre.code` around an unhighlighted block, and `pre.code.chroma`
  around a Chroma-highlighted one, each with a `language-*` class on the inner
  `code` element

### Classes the clients write

`search.js` builds `search-empty-state`, `search-empty-hint`, `search-quick-links`,
`search-results-list`, `search-result-item` with `selected`, `search-result-top`,
`search-result-section`, `search-result-sep`, `search-result-doc`,
`search-result-title`, `search-result-icon`, `search-result-snippet` and
`search-match`.

`diagrams.js` wraps each diagram in `mermaid-frame` with a `mermaid-toolbar`
carrying `mermaid-type`, `mermaid-actions`, `mermaid-copy` and `mermaid-expand`,
adds `mermaid-rendered`, and builds the lightbox out of `wiki-lbx` with its
`wiki-lbx-bar`, `wiki-lbx-id`, `wiki-lbx-pill`, `wiki-lbx-title`,
`wiki-lbx-ctrls`, `wiki-lbx-btn`, `wiki-lbx-stage`, `wiki-lbx-content` and
`wiki-lbx-hint`. A diagram that will not parse keeps `mermaid-fallback` on the
source element and gains a `mermaid-error` card with `mermaid-error-head`,
`mermaid-error-detail` and `mermaid-error-source`.

An unstyled class is invisible rather than broken, so a theme that misses one of
these loses the affordance, not the feature. The ids above are the ones that
break behaviour.

## Building a theme

Three sizes, cheapest first. All three are one directory.

**Recolour.** Copy the default's `tokens.css` into `assets/`, change the values
you care about, and declare one asset. Fonts, template and both clients are
inherited. This is the right answer nine times out of ten.

**Reskin.** The same, plus `wiki.css` and a `fonts/` subtree, and optionally
`brand-mark.svg` and `favicon.svg`. You keep the default's `template.html` and
`search.js`, so the DOM contract above stays satisfied for free.

**Relayout.** Replace `template.html` as well, or use slots to inject into the
default's shell without owning it. Read the DOM contract section first; this is
the only tier where you can break the clients.

A checklist before shipping one:

- Render it and open a page. The `theme.json` in the output records the resolved
  theme, which is the quickest way to confirm the override was picked up at all.
- Open the search modal and type. That exercises the index fetch, the base path
  and the result styling in one go.
- Click a diagram, open the lightbox, and pan it.
- Check both grounds. A theme that declares only light values inherits the
  default's dark ones, which is usually fine and occasionally wrong.
- Tab through the page. The focus ring is a 2px outline at a 2px offset, so a
  stray `outline: none` anywhere is a bug.
- Check 320px. Overflow at that width is easy to miss, and was a real failure in
  the design the default theme is drawn from.
- Confirm the focus ring, and any control whose only boundary is a border, clear
  3:1 against the surface they sit on.

## Recolouring without restating anything

Because a theme inherits, a recolour is one file. Copy the default's `tokens.css`
into `assets/`, change the values you care about, and declare nothing else:

```json
{ "name": "acme", "assets": { "tokens.css": { "mode": "replace" } } }
```

Everything else, including the fonts, the template and both clients, is inherited
byte for byte. This is the case the manifest's `inherit` mode exists for, and it
is why an override does not have to be a fork.

## The base path

`--base` moves the site. It is both the URL prefix and the path inside the output
directory, so a rendered directory can be dropped behind any static file server
unchanged: the pages are already where the links say they are. With
`--base /docs/deep/`, `--out out`, the pages are written under `out/docs/deep/`
and every asset, page link, the search index fetch, the mermaid bundle path and
the root redirect follow.

## Provenance

Every render writes a resolved `theme.json` next to the pages, recording the
theme's name, version and description, whether an override was applied, the
resolved asset list, the base, and the slot sizes. A published site is
self-describing, and a reader can tell which layer produced it.

## See also

- [`design/branding.md`](../design/branding.md): what a theme owns
- [`usage/container.md`](../usage/container.md): mounting a theme read-only
