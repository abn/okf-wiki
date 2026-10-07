---
type: Design
title: Branding and theming
description: The default theme, what a theme may change, and how an override is layered over it.
tags: [design, branding, theme]
---

# Branding and theming

okf-wiki ships with a theme called `paper`: a white ground, near-black ink, one
green accent, pill controls, and a two-face split where Figtree carries
everything read as language and Inter carries everything meant to be operated.
The default is embedded in the binary, so the binary alone renders a complete,
offline, self-contained site.

A theme is a directory, not a build step. `--theme DIR` layers a directory over
the embedded default, so re-skinning a wiki is a mount and an environment
variable rather than a rebuild and a new image.

## What a theme owns, and what it does not

A theme owns appearance: the stylesheet, the design tokens, the fonts, the
favicon and header mark, the shell template, and optionally the two client
scripts. It does not own the renderer. What a theme may not change is the
document contract:

- the class names and ids the clients look for, listed in the stylesheet's header
  comment
- the template fields listed below
- `search-index.json` and its shape
- the mermaid contract: `pre.mermaid` in, rendered SVG plus a lightbox out

That split is the point. A theme that only wants a different skin restyles the
class names and never touches `template.html`; a theme that wants a different
layout replaces the template and inherits the stylesheet.

## Brand configuration

Set by flag or environment variable, and independent of the theme:

- `--brand` / `OKF_WIKI_BRAND`: the wordmark. A trailing period is split out and
  handed to the template as a separate field, which a theme may paint in its
  accent.
- `--brand-sub` / `OKF_WIKI_BRAND_SUB`: the small label beside the wordmark.
- `--version-tag` / `OKF_WIKI_VERSION_TAG`: a freeform version label shown as a
  pill beside the wordmark, for a release tag, a build name, or anything else
  that names this deployment. Empty shows nothing.
- `--title` / `OKF_WIKI_TITLE`: the document title suffix.
- `--sections`: section order as comma-separated `id:Title` pairs.
- `--nav-links` / `OKF_WIKI_NAV_LINKS`: links in the header, comma-separated as
  `Label=URL` pairs. An external target opens in a new tab and is marked with an
  arrow; an in-bundle path resolves as a body link and opens in the same tab. On
  a phone the links move into the drawer, the in-wiki ones first under "On this
  wiki" and the rest under "Elsewhere".

## The template contract

`template.html` is `html/template` and may rely on any of these fields:

| Field | What it holds |
| :--- | :--- |
| `{{.Title}}` `{{.Description}}` | the page title and lead |
| `{{.Kicker}}` | an eyebrow label above the title, empty on ordinary pages |
| `{{.TitleHTML}}` | trusted H1 markup replacing the title when set, today only on synthetic pages |
| `{{.Body}}` `{{.TOC}}` `{{.Meta}}` `{{.Nav}}` `{{.Breadcrumb}}` | the rendered document, already trusted HTML |
| `{{.TopNav}}` | the header's links in configured order, empty when none are set |
| `{{.TopNavInternal}}` `{{.TopNavExternal}}` | the same links split by kind, for the drawer |
| `{{.TOCList}}` | the heading entries as a bare list, for a sidebar column or dropdown |
| `{{.PrevNext}}` | the previous/next cards for the section, empty when the page stands alone |
| `{{.Base}}` | the URL prefix, with a leading and trailing slash |
| `{{.SearchBase}}` | the URL prefix the search index is fetched from: the same as `Base` for a single bundle, the tree base for a wiki of a multi-site tree |
| `{{.AssetVersion}}` | the resolved theme's content hash |
| `{{.BrandName}}` `{{.BrandTail}}` `{{.BrandSub}}` `{{.BrandTitle}}` | the configured brand |
| `{{.VersionTag}}` | a freeform version label beside the wordmark, empty when unset |
| `{{.ThemeName}}` `{{.ThemeVersion}}` | the resolved theme's identity |
| `{{.QuickJSON}}` | the search modal's quick-link list, as a JS value |
| `{{.Slot "name"}}` | a named fragment from the manifest's `slots` |

A theme never hardcodes `/wiki/`. Every asset and page link hangs off
`{{.Base}}`, and the clients read the same value from `window.__WIKI_BASE`, so
`--base` relocates the whole site rather than only its markup. The search client
reads its index location from `window.__WIKI_SEARCH`, which equals
`window.__WIKI_BASE` for a single bundle and is the tree base for a wiki of a
multi-site tree, where one aggregate index spans every wiki. A theme that keeps
the default template gets both for free.

## Two accessibility fixes, deliberately not reproduced

The design this theme is drawn from has a documented focus ring at 2.52:1 on its
own page ground and a success colour 0.01 under the text floor on the one status
surface it actually renders. `paper` does not reproduce either, and the numbers
are in `tokens.css` next to the values. A third change is structural: that
design used one hairline for decorative card outlines and for control boundaries,
so a control's only boundary sat at 1.26:1. Here the decorative rule and the
control boundary are separate tokens and only the latter is held to 3:1.

## See also

- [`reference/themes.md`](../reference/themes.md): the manifest, the resolution
  order, and how to write one
- [`usage/container.md`](../usage/container.md): mounting a theme into the
  container
