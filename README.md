# okf-wiki

Render an Open Knowledge Format (OKF v0.2) bundle as a branded, searchable
static wiki, and serve it. Ships as a single static binary and as a container
that auto-detects a mounted content directory.

## Quick start

```bash
# Binary
make run                      # render ./docs and serve on http://127.0.0.1:8080/wiki/

# Container: mount any OKF bundle, no container path needed
podman run --rm --network=host -v "$(pwd)/docs:ro,Z" ghcr.io/abn/okf-wiki:latest
```

The `:Z` relabels the bind mount so an SELinux host lets the container read it.

## What you get

- Sidebar navigation, on-page table of contents, breadcrumbs.
- GitHub alert callouts, syntax-highlighted code, Mermaid diagrams (ELK layout)
  with a full-screen pan/zoom lightbox.
- Full-text search over a build-time index, opened with `Cmd/Ctrl-K` or `/`.
- A light/dark theme, skinnable at runtime by mounting a theme directory.
- A container that renders diagrams with no network access.

## Theming

The default theme is embedded in the binary. `--theme DIR` (or
`OKF_WIKI_THEME`) layers a directory over it, so re-skinning is a bind mount and
an environment variable rather than a rebuild:

```bash
podman run --rm -p 8080:8080 \
  -v "$(pwd)/docs:/content:ro" \
  -v "$(pwd)/themes/acme:/theme:ro,Z" \
  -e OKF_WIKI_THEME=/theme \
  ghcr.io/abn/okf-wiki:latest
```

A theme restates only what it changes. See
[docs/reference/themes.md](docs/reference/themes.md).

## GitHub Actions

```yaml
- uses: abn/okf-wiki/action@v1
  with:
    content: docs
    deploy: true    # omit to build only
```

The base path comes from the repository name, so a project site publishes under
`/<repo>` unconfigured. See
[docs/usage/github-action.md](docs/usage/github-action.md).

## Documentation

The project wiki lives in [`docs/`](docs/index.md) and is rendered by this tool.
Read it at `make run`, or start with [docs/index.md](docs/index.md).

## Development

```bash
make check        # format, vet, test
make render       # render docs/ to .scratch/wiki
make vendor       # build the offline mermaid bundle (needs node)
make container/build
```

See [docs/contribution/index.md](docs/contribution/index.md) for the working
agreement.

## Licensing

No project LICENSE has been chosen yet. Third-party components (self-hosted
fonts and the bundled diagram runtime) and their licenses are recorded in
[THIRD-PARTY.md](THIRD-PARTY.md).
