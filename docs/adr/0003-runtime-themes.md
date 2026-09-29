---
type: Decision
title: Themes are runtime directories, not a compiled-in skin
description: Why okf-wiki resolves a theme from a directory at render time instead of embedding the brand in the binary.
tags: [adr, decision, theme]
status: accepted
---

# 0003 Themes are runtime directories

## Context

The first version compiled the whole visual system into the binary. A
`//go:embed assets` carried the stylesheet, the tokens, the fonts and both
client scripts, and the page shell was a Go string constant. The brand, abn.is,
was part of the program rather than configuration, so changing a colour meant
editing Go-tree assets, rebuilding, and repushing the image.

Three consequences were not obvious from the code.

The asset version every page links as `?v=` was computed by hashing a hardcoded
list of files read from the embed, so it never saw a theme override and an
overridden stylesheet could not invalidate a cached one. The override was a
blind `filepath.Walk` copy, so a theme had to restate every file or ship a
broken subset. And `/wiki/` was hardcoded in four places in Go and once in each
client, which made the site impossible to relocate.

## Decision

A theme is a directory, resolved once before rendering, layered over a default
that is compiled into the binary.

`theme.json` declares per asset whether it is `inherit`, which is the default,
`replace`, or `drop`. A key ending in a slash is a subtree, so swapping a font
directory is one line rather than ten filenames. The shell template is
replaceable, and a theme that only wants to inject uses the named `head` and
`bodyEnd` slots instead of owning the shell.

The asset version fingerprints the resolved bytes, so a theme edit busts the
cache. `--base` is both the URL prefix and the path inside the output directory,
so a rendered tree serves from any static host unchanged.

## Consequences

The default theme stays a build-time input: changing `paper` means editing
`internal/wiki/themes/default` and rebuilding. That is deliberate. The default
is the fallback layer, and an override that replaces one stylesheet inherits the
fonts, the template and both clients from something that has to exist whether or
not an override is present. Externalising it would mean a wiki with no theme
directory on disk cannot render at all, and the container would ship no skin. The
cost is about 190 KB, half of it fonts, inside the binary.

The escape hatch is cheap if that trade ever stops being right: drop the embed
and resolve the default from an executable-relative directory, ordered `--theme`,
then a conventional path, then the binary's own directory. It costs the
single-binary invariant.

The abn.is theme lives outside this repository. It is brand-specific and is not
published here, and a mount plus `OKF_WIKI_THEME` is all it takes to use it.

## Alternatives considered

**Copy the whole theme per deployment.** Simpler, and what the old override did.
Rejected because a theme then restates files it did not change, and the two
copies drift apart silently.

**A registry or URL transport, so themes can be pulled rather than mounted.**
Deferred rather than rejected. It puts a network call in a render that is
currently hermetic, and it is not needed for the case that matters: a deployment
already mounts a content directory, so mounting a theme beside it costs nothing.
A mount is also the only transport that works with a read-only root filesystem.

**goreleaser.** Rejected for images, because the publish workflow already builds
linux/amd64 and linux/arm64 natively and assembles a manifest list with podman,
and the house rule is podman throughout. Rejected for releases too: with no
tagged releases, users consume commit-id images and
`go install ...@latest` resolves to a pseudo-version. Worth revisiting if binary
archives are ever wanted, scoped to archives so it does not duplicate publishing.

## See also

- `design/branding.md` for what a theme owns and what it does not
- `reference/themes.md` for the manifest and the DOM contract
