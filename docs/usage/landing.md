---
type: Guide
title: Landing pages
description: Introducing a deployment with an optional landing.md rendered in the same theme.
tags: [usage, landing]
---

# Landing pages

A bundle root holding `landing.md` introduces the deployment at `/` instead of
redirecting to the wiki. The page renders through the same shell, header,
tokens and search as every wiki page, with no sidebar, so the layout collapses
to its wide solo column. Absent the file, the output root keeps redirecting to
the wiki base exactly as before.

```text
docs/
  landing.md     the introduction, rendered to the output root
  index.md       the wiki home, rendered under the base as usual
  guide/         sections, unchanged
```

With `--base /wiki/`, `/` serves the landing and `/wiki/` serves the wiki.
Serving the output directory lands on the introduction; the wiki is one click
past it.

## Writing one

`landing.md` is ordinary Markdown with OKF frontmatter. The title and
description render as the page heading and lead, and `kicker` renders an
eyebrow label above the title:

```markdown
---
title: Example
description: A friendlier front end to Example service.
kicker: One binary · no runtime
---

Start here, then add rich sections below with Markdown and HTML.
```

A `type` or `tags` frontmatter renders chips under the lead the way a wiki
page shows them. There is no per-page download link: the page is synthetic and
has no wiki URL of its own.

Links resolve from the bundle root, the way header links do, so prefer
root-relative targets (`/guide/setup.md`) over relative ones that move with
the file. In a group the root is the tree root rather than the group
directory, so the same preference applies doubly there. Every non-Markdown
file beside the page is copied into the site, so images and other assets
travel with it, in a group as much as in a single bundle.

## Rich sections

The theme ships `landing.css` with namespaced blocks for the sections a
one-stop shop tends to need: `landing-cta` with `landing-btn` and
`landing-btn-primary`, `landing-grid` with `landing-card`, the
`landing-grammar` phase strip, the `landing-code` command strip,
`landing-tabs` with `landing-pane` panels driven by a small inline script,
`table.landing-cmp` for comparisons, and `landing-qs` with `landing-step` for
quickstarts. Restyle them in a theme; the names are the contract.

## Groups in a tree

In [multi-site mode](multi-site.md) a group directory holding `landing.md`
shows its body above the generated catalog, so the group index reads as one
page: the authored introduction first, then the wiki cards. The landing's
frontmatter names the group when no `.meta.json` does. Inside a wiki directory
the name stays reserved and inert: it is never published as a page, and the
wiki's own `index.md` owns the title there. A content root that is itself a
wiki ignores the file for the same reason; the wiki home already owns the base.

## Limits

The name `landing.md` is reserved bundle-wide and skipped as a page at any
other depth. A base of `/` leaves no room for both a wiki home and a landing
page at the output root, so that combination fails the render and names the
clash; serve the wiki under a base such as `/wiki/` to make room. Adding or
removing the file under `--watch` takes effect on the next load with no
restart.
