#!/usr/bin/env python3
"""Render the GitHub social preview cards.

Usage: capture-social-card.py [--out DIR] [--chrome PATH]

scripts/social-card/card.html is the source. Each theme is rendered at
1280x640, the size GitHub asks for and the largest it will store, and written
under .github/assets/social. The script needs a Chrome or Chromium build; pass
one with --chrome or CHROME when it is not on the PATH.

Nothing is replaced until both themes have rendered and passed the checks, so a
failed run leaves the committed pair untouched. The checks are deliberately
strict: a card whose mark or fonts did not load, or that is missing, blank, the
wrong size, not a PNG, or over GitHub's 1 MB limit, is rejected here rather than
at upload time.
"""

from __future__ import annotations

import argparse
import os
import shutil
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CARD = ROOT / "scripts" / "social-card" / "card.html"
DEFAULT_OUT = ROOT / ".github" / "assets" / "social"

WIDTH, HEIGHT = 1280, 640
MAX_BYTES = 1024 * 1024
# A blank capture at this size compresses to about 4 KB; a card with type and
# the mark lands several times higher.
MIN_BYTES = 8 * 1024
THEMES = ("light", "dark")
TIMEOUT = 90

CHROME_NAMES = ("google-chrome-stable", "google-chrome", "chromium", "chromium-browser")


def is_executable(path: str) -> bool:
    return Path(path).is_file() and os.access(path, os.X_OK)


def find_chrome(explicit: str | None) -> str:
    for candidate in (explicit, os.environ.get("CHROME")):
        if not candidate:
            continue
        if not is_executable(candidate):
            sys.exit(f"not an executable Chrome: {candidate}")
        return candidate
    for name in CHROME_NAMES:
        found = shutil.which(name)
        if found:
            return found
    sys.exit("no Chrome or Chromium found; pass --chrome PATH or set CHROME")


def run_chrome(chrome: str, theme: str, extra: list[str]) -> str:
    url = CARD.as_uri()
    if theme == "dark":
        url += "?theme=dark"
    with tempfile.TemporaryDirectory(
        prefix="okf-wiki-social-", ignore_cleanup_errors=True
    ) as profile:
        cmd = [
            chrome,
            "--headless=new",
            "--disable-gpu",
            "--hide-scrollbars",
            "--no-first-run",
            "--no-default-browser-check",
            "--force-device-scale-factor=1",
            "--run-all-compositor-stages-before-draw",
            f"--window-size={WIDTH},{HEIGHT}",
            "--virtual-time-budget=3000",
            f"--user-data-dir={profile}",
            *extra,
            url,
        ]
        try:
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=TIMEOUT)
        except subprocess.TimeoutExpired:
            sys.exit(f"chrome timed out after {TIMEOUT}s on the {theme} card")
    if result.returncode != 0:
        sys.exit(f"chrome failed on the {theme} card:\n{result.stderr.strip()}")
    return result.stdout


def render(chrome: str, theme: str, out: Path) -> None:
    run_chrome(chrome, theme, [f"--screenshot={out}"])


def check_assets(chrome: str, theme: str) -> None:
    dom = run_chrome(chrome, theme, ["--dump-dom"])
    if "data-asset-error" in dom:
        sys.exit(f"an asset failed to load in the {theme} card; check the paths in card.html")
    if "data-font-error" in dom:
        sys.exit(f"a font failed to load in the {theme} card; check the imported fonts.css")


def verify(path: Path) -> int:
    if not path.is_file():
        sys.exit(f"no image written for the {path.name} card")
    data = path.read_bytes()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        sys.exit(f"{path} is not a PNG")
    width, height = struct.unpack(">II", data[16:24])
    if (width, height) != (WIDTH, HEIGHT):
        sys.exit(f"{path} is {width}x{height}, expected {WIDTH}x{HEIGHT}")
    if len(data) < MIN_BYTES:
        sys.exit(f"{path} is {len(data)} bytes, which is a blank page")
    if len(data) > MAX_BYTES:
        sys.exit(f"{path} is {len(data)} bytes, over GitHub's 1 MB limit")
    return len(data)


def display(path: Path) -> str:
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--out", default=str(DEFAULT_OUT), help="output directory")
    parser.add_argument("--chrome", help="path to a Chrome or Chromium binary")
    args = parser.parse_args()

    if not CARD.is_file():
        sys.exit(f"missing card source: {CARD}")

    chrome = find_chrome(args.chrome)
    out_dir = Path(args.out).expanduser().resolve()
    out_dir.mkdir(parents=True, exist_ok=True)

    rendered: list[tuple[Path, int]] = []
    with tempfile.TemporaryDirectory(dir=out_dir, prefix=".render-") as stage:
        for theme in THEMES:
            staged = Path(stage) / f"okf-wiki-social-{theme}.png"
            print(f"rendering the {theme} card")
            check_assets(chrome, theme)
            render(chrome, theme, staged)
            rendered.append((staged, verify(staged)))

        for staged, size in rendered:
            out = out_dir / staged.name
            os.replace(staged, out)
            print(f"wrote {display(out)} ({WIDTH}x{HEIGHT}, {size // 1024} KB)")


if __name__ == "__main__":
    main()
