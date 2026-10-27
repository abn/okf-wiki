#!/usr/bin/env python3
"""Capture the README screenshots from a local render.

Usage: capture-readme-screenshots.py [--out DIR] [--site DIR] [--chrome PATH]
                                     [--skip-render]

The script renders ./docs, serves the output on a loopback port, and writes the
images under .github/assets/readme. It needs Playwright and a Chromium build. A
system Chrome is used when Playwright's own browser is not installed, and
--chrome points at one explicitly.

The images are ordinary PNGs. When ImageMagick is available they are reduced to
a 256 colour palette, which keeps the repository's pre-commit file size gate
comfortable without changing how the UI looks.
"""

from __future__ import annotations

import argparse
import functools
import http.server
import os
import shutil
import subprocess
import sys
import threading
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_OUT = ROOT / ".github" / "assets" / "readme"
DEFAULT_SITE = ROOT / ".scratch" / "readme-site"

DESKTOP = {"width": 1600, "height": 1000}
PHONE = {"width": 390, "height": 844}

BRANDING = [
    "--brand", "okf-wiki",
    "--brand-sub", "docs",
    "--title", "Wiki",
    "--base", "/wiki/",
    "--bundle",
]


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args: object) -> None:
        pass


def render(site: Path) -> None:
    bundle = ROOT / "mermaid" / "mermaid-bundle.min.mjs"
    if not bundle.is_file():
        sys.exit("mermaid/mermaid-bundle.min.mjs is missing; run make vendor first")

    env = os.environ.copy()
    env.setdefault("GOCACHE", str(ROOT / ".scratch" / "gocache"))
    cmd = [
        "go", "run", "./cmd/okf-wiki", "render",
        "--content", "docs",
        "--out", str(site),
        *BRANDING,
    ]
    print("rendering docs for screenshots")
    subprocess.run(cmd, cwd=ROOT, env=env, check=True)


def serve(site: Path) -> tuple[http.server.ThreadingHTTPServer, str]:
    handler = functools.partial(QuietHandler, directory=str(site))
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    port = server.server_address[1]
    return server, f"http://127.0.0.1:{port}/wiki/"


def launch_browser(playwright, chrome: str | None):
    if chrome:
        return playwright.chromium.launch(executable_path=chrome)

    try:
        return playwright.chromium.launch()
    except Exception:
        for name in ("google-chrome", "chromium", "chromium-browser"):
            path = shutil.which(name)
            if path:
                return playwright.chromium.launch(executable_path=path)
        sys.exit(
            "no Chromium found; run playwright install chromium or pass --chrome PATH"
        )


def settle(page) -> None:
    page.wait_for_load_state("networkidle")
    page.evaluate("document.fonts.ready")
    page.wait_for_timeout(400)


def capture(page, out: Path, name: str) -> None:
    path = out / name
    page.screenshot(path=str(path), full_page=False)
    print(f"  {path.relative_to(ROOT)}  {path.stat().st_size // 1024} KB")


def optimise(out: Path) -> None:
    tool = shutil.which("magick") or shutil.which("convert")
    if not tool:
        return

    for path in sorted(out.glob("*.png")):
        temporary = path.with_name(path.stem + ".opt.png")
        try:
            subprocess.run(
                [tool, str(path), "-strip", "-colors", "256", f"PNG8:{temporary}"],
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        except subprocess.CalledProcessError:
            temporary.unlink(missing_ok=True)
            continue
        temporary.replace(path)
        print(f"  {path.relative_to(ROOT)}  {path.stat().st_size // 1024} KB")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    parser.add_argument("--site", type=Path, default=DEFAULT_SITE)
    parser.add_argument("--chrome", help="path to a Chrome or Chromium binary")
    parser.add_argument("--skip-render", action="store_true")
    args = parser.parse_args()

    out = args.out.resolve()
    site = args.site.resolve()
    out.mkdir(parents=True, exist_ok=True)

    if not args.skip_render:
        render(site)
    elif not (site / "wiki" / "index.html").is_file():
        sys.exit(f"{site} has no render; run without --skip-render first")

    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        sys.exit("playwright is not installed; run pip install playwright")

    server, base = serve(site)
    try:
        with sync_playwright() as playwright:
            browser = launch_browser(playwright, args.chrome)
            context = browser.new_context(
                viewport=DESKTOP,
                device_scale_factor=1,
                reduced_motion="reduce",
                color_scheme="light",
            )
            page = context.new_page()

            page.goto(base + "design/rendering.html")
            settle(page)
            page.wait_for_selector("pre.mermaid svg", timeout=15000)
            capture(page, out, "hero-light.png")

            modifier = "Meta" if sys.platform == "darwin" else "Control"
            page.keyboard.press(f"{modifier}+k")
            page.wait_for_selector("#searchBackdrop.open")
            page.fill("#searchInput", "mermaid")
            page.wait_for_selector(".search-result-item")
            page.wait_for_timeout(300)
            capture(page, out, "search.png")

            page.goto(base + "graph.html")
            settle(page)
            page.wait_for_selector("#graphNodes > *")
            before = page.inner_text("#graphStatus")
            page.uncheck("#graphTagHubs")
            page.wait_for_function(
                "before => document.getElementById('graphStatus').textContent !== before",
                arg=before,
            )
            page.click("#graphFit")
            page.wait_for_timeout(500)
            page.evaluate("document.activeElement.blur()")
            capture(page, out, "graph.png")

            page.goto(base + "testbed/callouts.html")
            settle(page)
            capture(page, out, "theme-light.png")
            page.click('button[aria-label="Toggle light and dark theme"]')
            page.wait_for_timeout(500)
            capture(page, out, "theme-dark.png")

            context.close()

            phone = browser.new_context(
                viewport=PHONE,
                device_scale_factor=2,
                is_mobile=True,
                has_touch=True,
                reduced_motion="reduce",
                color_scheme="light",
            )
            page = phone.new_page()
            page.goto(base + "design/rendering.html")
            settle(page)
            page.wait_for_selector("pre.mermaid svg", timeout=15000)
            capture(page, out, "mobile.png")
            phone.close()

            browser.close()
    finally:
        server.shutdown()
        server.server_close()

    optimise(out)
    print(f"screenshots written to {out.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
