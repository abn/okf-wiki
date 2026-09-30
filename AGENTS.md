# AGENTS.md

Committed entrypoint for humans and agents working in this repository.

## Project

okf-wiki renders an OKF v0.2 bundle into a static, branded, searchable wiki and
serves it, as a binary or a container. The design lives in
[`docs/`](docs/index.md); start at [`docs/overview.md`](docs/overview.md).

## Invariants

- No sudo, and no elevation, unless a human explicitly asks for it.
- The committed `docs/` bundle is public. No hostnames, absolute home paths,
  tokens, or internal identifiers.
- Prose reads as human-written. No em-dashes, no marketing filler, no comments
  that restate the code. Full comment rules in
  [`.agents/rules/comments.md`](.agents/rules/comments.md).
- A behaviour change moves the docs with it: update the relevant page and add a
  line to [`docs/log.md`](docs/log.md).
- The `docs/` bundle is the single source of truth. Never hand-edit rendered
  output (`.scratch/`, `wiki/`); regenerate it.

## Conventions

- Commits follow Conventional Commits, summary first, no trailers.
- Stage explicit paths. Do not use `git add -A`.
- Keep changes scoped: state the scope in one sentence first, and stay inside it.
- Refer to a container image by its fully qualified name everywhere, including
  prose, examples, the Makefile, the `FROM` lines in the `Containerfile`, and its
  `# syntax=` directive. Our images are `ghcr.io/abn/<repo>:<tag>`; Docker Hub
  base images are `docker.io/library/<repo>:<tag>`. Never the short
  `okf-wiki:latest` or `alpine:3.20` forms, which resolve against whatever
  registry happens to be configured and silently pull the wrong thing.

## Automation

- `make check` is the quality gate: `gofmt`, `go vet`, `go test`.
- `make render`, `make run`, `make vendor`, and `make container/build` cover the
  render, serve, offline bundle, and image paths.

## Verification

A change is done only when `make check` passes and the relevant behaviour was
exercised with real captured output. For rendering or container changes, render
a bundle and confirm the pages, search index, and diagrams are produced.

## Contributor guide

See [`docs/contribution/index.md`](docs/contribution/index.md).

## Agent rules

- [`.agents/rules/comments.md`](.agents/rules/comments.md): what a comment may
  say, and what it must never say.
- [`.agents/rules/repository-conventions.md`](.agents/rules/repository-conventions.md):
  the public docs bundle, generated output, offline diagrams, and scope
  discipline.
