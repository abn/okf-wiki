#!/usr/bin/env bash
# Idempotent bootstrap for local tooling. Committed and safe to re-run.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

# Use pre-commit for git hooks, not core.hooksPath, so the committed
# .pre-commit-config.yaml is the single source of truth.
git config --unset core.hooksPath 2>/dev/null || true

if command -v pre-commit >/dev/null 2>&1; then
  pre-commit install --install-hooks
  pre-commit install --hook-type commit-msg
else
  printf 'okf-wiki: pre-commit not found, skipping hook install\n' >&2
fi

mkdir -p .scratch/gocache

printf 'okf-wiki: bootstrap complete\n'
