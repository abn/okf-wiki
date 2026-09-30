# Contribution

## Working agreement

- State the scope of a change in one sentence before starting.
- Keep changes minimal and in service of that scope.
- Run `make check` before claiming a change is done. It must pass with real
  captured output.
- Commit in small, logical units using Conventional Commits.

## Documentation

The `docs/` bundle is the project wiki and is public. It must never contain
hostnames, absolute home paths, tokens, or internal identifiers. When behaviour
changes, update the relevant page and add an entry to `docs/log.md`.

## Environment

- Go 1.25 or newer.
- Node 22 and npm, only to build the offline mermaid bundle (`make vendor`) or
  the container image.

## Layout

- `cmd/okf-wiki`: the CLI.
- `internal/wiki`: renderer, config, detection, server, and embedded assets.
- `mermaid`: the pinned mermaid and ELK bundle build.
- `Containerfile`: the multi-stage image build.

## Local hooks

Install the committed pre-commit hooks once per clone:

```bash
pre-commit install --install-hooks
```

They mirror CI: whitespace and EOF fixes, a YAML check, a large-file guard,
`make check` (format, vet, tests), a markdown em-dash guard, and Conventional
Commits on the commit message. Run them over the whole tree with
`pre-commit run --all-files`.

## Continuous integration

GitHub Actions drives quality and release. The workflows live in
`.github/workflows/`.

`ci.yml` runs on every push to `main` and every pull request:

- **go**: `gofmt`, `go vet`, `go test -race`, and a binary build.
- **vendor**: builds the offline mermaid bundle with the pinned lockfile.
- **container-build**: builds the image with podman, then runs it and checks
  `/healthz` and the wiki page.
- **render**: renders `docs/` and checks the pages and search index exist.
- **pre-commit**: runs every hook over the tree.

`publish.yml` runs on `main`, version tags, and manual dispatch. It builds the
image natively on `linux/amd64` and `linux/arm64` runners with podman, pushes
each as a per-arch tag, then assembles and pushes one multiarch manifest:

```bash
podman pull ghcr.io/abn/okf-wiki:latest
```

Registry and image name are set by the `IMAGE_NAME` repository variable
(a full reference without a tag, for example `ghcr.io/abn/okf-wiki`); the login
registry is derived from it. Publishing uses the workflow `GITHUB_TOKEN`, so no
extra secrets are required.

## Dependency updates

Dependabot (`.github/dependabot.yml`) opens grouped updates once a month for Go
modules, GitHub Actions, the mermaid npm bundle, and the container base images.
Each ecosystem lands as a single pull request, so a month of bumps arrives as
one reviewable change rather than a stream.
