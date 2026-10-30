#!/usr/bin/env python3
"""Measure the planner against a repository's own page history.

Usage: evaluate-similar.py --repo DIR --bundle PATH [--bin PATH] [--updates N]
                          [--new N] [--since DATE]

For each sampled commit the script exports the bundle at the parent commit,
turns the commit's added lines into a draft, and asks `okf-wiki similar` where
that draft belongs. It reports recall at 1 and 3 for edits to subject pages,
keeps edits to a section index (`index.md`) in a separate row because a hub is
not a subject, and reports the share of genuinely new subjects the planner
leaves as new.

The point is the threshold, not the number: a bundle whose pages are written
differently will land on a different default. Nothing in the repository is
modified; each export goes to a temporary directory and is removed at the end.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile

ROLES = {
    "index", "overview", "report", "evidence", "abstract", "method",
    "implications", "implementations", "benchmark", "claims", "limits",
    "risks", "setup", "usage", "comparison", "discovery", "history",
    "quirks", "integration", "capabilities", "architecture",
}


def run(*args, text=True):
    return subprocess.run(args, capture_output=True, text=text, check=False)


def subject_of(bundle: str, rel: str) -> str:
    slug = rel[len(bundle) + 1:] if rel.startswith(bundle + "/") else rel
    slug = slug[:-3] if slug.endswith(".md") else slug
    directory, _, base = slug.rpartition("/")
    if directory and base in ROLES:
        return directory
    return slug


def is_section_index(rel: str) -> bool:
    return os.path.basename(rel) == "index.md"


def added_text(repo: str, sha: str, rel: str) -> str:
    diff = run("git", "-C", repo, "diff", f"{sha}^", sha, "--", rel).stdout
    out = [line[1:] for line in diff.splitlines()
           if line.startswith("+") and not line.startswith("+++")]
    if out and out[0].strip() == "---":
        for i, line in enumerate(out[1:], 1):
            if line.strip() == "---":
                out = out[i + 1:]
                break
    return "\n".join(out)


def export_parent(repo: str, sha: str, bundle: str, dest: str) -> str | None:
    archive = run("git", "-C", repo, "archive", f"{sha}^", bundle, text=False)
    if archive.returncode != 0:
        return None
    with tempfile.NamedTemporaryFile(suffix=".tar") as f:
        f.write(archive.stdout)
        f.flush()
        with tarfile.open(f.name) as t:
            t.extractall(dest, filter="data")
    path = os.path.join(dest, bundle)
    return path if os.path.isdir(path) else None


def plan(binary: str, content: str, draft: str) -> dict | None:
    with tempfile.NamedTemporaryFile("w", suffix=".md", delete=False) as f:
        f.write(draft)
        name = f.name
    try:
        p = run(binary, "similar", "--content", content, "--text", name, "--json", "--top", "10")
        return json.loads(p.stdout) if p.returncode == 0 else None
    finally:
        os.unlink(name)


def summarize(label: str, ranks: list[int]) -> None:
    if not ranks:
        return
    n = len(ranks)
    print(f"{label} n={n} recall@1={sum(1 for r in ranks if r == 1)/n:.2f} "
          f"recall@3={sum(1 for r in ranks if 0 < r <= 3)/n:.2f} "
          f"miss={sum(1 for r in ranks if r == 0)/n:.2f}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--repo", required=True, help="git repository holding the bundle")
    ap.add_argument("--bundle", required=True, help="bundle path inside the repository")
    ap.add_argument("--bin", default="okf-wiki", help="okf-wiki binary (default: on PATH)")
    ap.add_argument("--updates", type=int, default=25, help="edit samples (default: 25)")
    ap.add_argument("--new", type=int, default=10, help="new-subject samples (default: 10)")
    ap.add_argument("--since", default="2026-09-20", help="git log --since value")
    args = ap.parse_args()

    commits = run("git", "-C", args.repo, "log", "--format=%H",
                  f"--since={args.since}", "--", args.bundle).stdout.split()
    if not commits:
        print("no commits in range", file=sys.stderr)
        return 2
    print(f"range {commits[-1][:12]}..{commits[0][:12]} since {args.since}, "
          f"{len(commits)} commits")

    updates, news = [], []
    for sha in commits:
        status = run("git", "-C", args.repo, "show", "--name-status", "--format=",
                     sha, "--", args.bundle).stdout
        for line in status.splitlines():
            parts = line.split("\t")
            if len(parts) != 2:
                continue
            kind, rel = parts
            if not rel.endswith(".md") or "/" not in rel:
                continue
            if rel.endswith("/log.md") or "/logs/" in rel:
                continue
            if kind == "M" and len(updates) < args.updates:
                draft = added_text(args.repo, sha, rel)
                if len(draft.split()) < 80:
                    continue
                updates.append((sha, rel, draft))
            elif kind == "A" and len(news) < args.new:
                directory = os.path.dirname(rel)
                if not directory:
                    continue
                if run("git", "-C", args.repo, "ls-tree", f"{sha}^", directory).stdout.strip():
                    continue
                draft = run("git", "-C", args.repo, "show", f"{sha}:{rel}").stdout
                if len(draft.split()) < 120:
                    continue
                news.append((sha, rel, draft))
        if len(updates) >= args.updates and len(news) >= args.new:
            break

    exports: dict[str, str | None] = {}
    tempdirs: list[str] = []

    def content_for(sha: str) -> str | None:
        if sha not in exports:
            dest = tempfile.mkdtemp(prefix="okf-eval-")
            tempdirs.append(dest)
            exports[sha] = export_parent(args.repo, sha, args.bundle, dest)
        return exports[sha]

    subject_ranks, index_ranks = [], []
    for sha, rel, draft in updates:
        content = content_for(sha)
        if not content:
            continue
        resp = plan(args.bin, content, draft)
        if not resp:
            continue
        want = subject_of(args.bundle, rel)
        got = [c["subject"] for c in resp["candidates"]]
        rank = got.index(want) + 1 if want in got else 0
        if is_section_index(rel):
            index_ranks.append(rank)
        else:
            subject_ranks.append(rank)
        score = resp["candidates"][0]["score"] if resp["candidates"] else 0
        print(f"rank={rank} score={score:.3f} verdict={resp['verdict']:11s} "
              f"kind={'index' if is_section_index(rel) else 'subject'} "
              f"want={want} top={got[:3]}")

    print()
    summarize("subject edits", subject_ranks)
    summarize("section index edits", index_ranks)

    left_new = 0
    for sha, rel, draft in news:
        content = content_for(sha)
        if not content:
            continue
        resp = plan(args.bin, content, draft)
        if not resp:
            continue
        ok = resp["verdict"] == "new_subject"
        left_new += ok
        top = resp["candidates"][0] if resp["candidates"] else {"score": 0, "subject": "-"}
        print(f"new_subject={ok} top={top['score']} want={subject_of(args.bundle, rel)} "
              f"got={top['subject']}")
    if news:
        print(f"new subjects n={len(news)} left as new={left_new/len(news):.2f}")

    for d in tempdirs:
        shutil.rmtree(d, ignore_errors=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
