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
changes, update the relevant page and add an entry page under `docs/logs/YYYY/MM/`.

The entry pages are the change log; `docs/log.md` explains the model and lists no
entries. Copy the most recent entry for its frontmatter, give it a short title under
80 characters, name the page for the day and the change, and link the pages the
change touched from its body. A new month also needs an `index.md` whose heading
names it, for example `# November`, so the rail and the month page carry the name
rather than the directory number.

## Environment

- Go 1.25 or newer.
- Node 22 and npm, only to build the offline mermaid bundles (`make vendor`)
  or the container image.
- Python 3 and Selenium, with Firefox, only to run `make firefox` or the CI
  Firefox smoke test.
- Playwright, with a Chromium build, only to run `make screenshots`.
- Chrome or Chromium, only to run `make social-card`. `--chrome` or `CHROME`
  points at one outside the PATH.

## Offline browser check

`make firefox` builds the diagram bundles, renders `docs/` as an offline folder,
and runs `scripts/check-offline-firefox.py` against it. It needs Firefox and
Selenium; Selenium Manager fetches a matching geckodriver. CI runs the same
target.

## README screenshots

`make screenshots` renders `docs/`, serves it on a loopback port, and writes the
images under `.github/assets/readme/` with
`scripts/capture-readme-screenshots.py`. It needs Playwright and a Chromium
build, or a system Chrome passed with `--chrome`. The images are ordinary PNGs
and are reduced to a palette when ImageMagick is available. Re-run the target
after a theme or rendering change so the front page does not show an old build.

## Social preview cards

`make social-card` renders `scripts/social-card/card.html` at 1280x640 and
writes the light and dark cards under `.github/assets/social/` with
`scripts/capture-social-card.py`. It needs Chrome or Chromium. The page imports
the theme's `tokens.css` and `fonts.css` and shows the committed brand mark, so
a palette, font or mark change reaches the cards on the next render. GitHub
takes one image in the repository settings, and the pair keeps both grounds
ready.

## Layout

- `cmd/okf-wiki`: the CLI.
- `internal/wiki`: renderer, config, detection, server, and embedded assets.
- `mermaid`: the pinned mermaid and ELK bundle build.
- `scripts`: browser smoke tests and the image capture scripts.
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
- **vendor**: builds the offline mermaid bundles with the pinned lockfile, one
  ESM build and one classic build for `file://`.
- **container-build**: builds the image with podman, then runs it and checks
  `/healthz` and the wiki page.
- **render**: renders `docs/` and checks the pages and search index exist.
- **firefox**: renders an offline folder and runs the Firefox `file://` smoke
  test for search, diagrams, the graph and the root redirect.
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

## Releases

Releases are driven by Conventional Commits and
[Release Please](https://github.com/googleapis/release-please).

`release-please.yml` runs on every push to `main`, works out the next version
from the commits since the last tag, and opens or updates a release pull
request. That pull request bumps the `Version` constant in
`cmd/okf-wiki/main.go` and adds the changelog entry. Merging it is the release:
Release Please creates the `vX.Y.Z` tag and a **draft** GitHub release, starting
at `1.0.0` for the first one.

The draft is deliberate. Releases here are immutable: once one is published, its
tag and its assets are locked, and GitHub rejects a later upload with
`HTTP 422: Cannot upload assets to an immutable release`. Every archive therefore
has to be attached while the release is still a draft, and only the last step
publishes it. The config sets `force-tag-creation`, so the tag exists as soon as
the draft does and the binaries workflow can check it out.

The release pull request and the release are both made with the workflow
`GITHUB_TOKEN`, and events from that token start no other workflows. The release
job therefore runs the follow-up work itself, in this order:

- It calls `binaries.yml`, which builds the static binary for linux, macOS and
  Windows on amd64 and arm64 with `CGO_ENABLED=0`, builds the mermaid runtime
  with the same Node commands as the image, and packs each archive with the
  binary, `LICENSE`, `README.md`, `THIRD-PARTY.md` and `mermaid/`. It writes
  `checksums.txt` and attaches the lot to the draft release, then compares the
  attached names and sizes against the files it built, because a missed file
  cannot be added after publication. The native archive is run before upload and
  must report the tag's version. The file takes a `tag` input, so archives can be
  attached again by dispatching it at a release that is still a draft.
- It publishes the draft, which is the point the tag and the archives become
  immutable and the release gets its attestation.
- It calls `semver.yml`, which runs
  [`actions-semver-checker`](https://github.com/jessehouwing/actions-semver-checker)
  with auto-fix. The checker keeps the references the action is consumed
  through correct: `vX` and `vX.Y` point at the newest patch release, every
  `vX.Y.Z` tag has a published release, and the newest release is marked latest.
  It runs after the publish step because the checker counts a draft as an
  unpublished release. Marketplace checks are off, because
  `action.yml` lives under `action/` and the repository is not a Marketplace
  listing. The same file runs on a published release and on manual dispatch, so
  a tag pushed or released by other means is repaired too.
- It dispatches `publish.yml` at the new tag, which publishes
  `ghcr.io/abn/okf-wiki:vX.Y.Z`. The tag trigger in `publish.yml` still covers a
  tag pushed by hand.

If a step fails before the release is published, the release stays a draft and
nothing is lost. Re-running the failed jobs retries it as it was, so it covers a
failure outside the repository, not a fix pushed afterwards: a re-run replays the
same commit and workflow file. For that, dispatch `binaries.yml` with the tag,
publish with `gh release edit vX.Y.Z --draft=false`, then dispatch `semver.yml`
and `publish.yml` at the tag. Re-running every job does not help, because the
release pull request is already spent: Release Please creates nothing and the
dependent jobs stay skipped. Leaving the draft is no better, since the semver
checker's auto-fix publishes any draft it finds, and this one would then be
published without its archives.

A release that was already published before this order was in place cannot gain
archives; the tag cannot be reused either, so the next release carries them.

A release pull request is opened by the GitHub Actions app, so its
`pull_request` runs are created in `action_required`: nothing executes until
someone chooses "Approve and run", and every force push Release Please makes to
the branch needs approving again. A run still waiting when the branch is deleted
at merge can end as a failure with no jobs, which is noise rather than a red
build. Approve the latest run when you want CI on the release pull request, or
merge it without checks: it holds the version bump and the changelog, and the
commits it gathers were already verified on `main`.

## Dependency updates

Dependabot (`.github/dependabot.yml`) opens grouped updates once a month for Go
modules, GitHub Actions, the mermaid npm bundle, and the container base images.
Each ecosystem lands as a single pull request, so a month of bumps arrives as
one reviewable change rather than a stream.
