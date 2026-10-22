#!/usr/bin/env python3
"""Smoke test an --offline render in Firefox over file://.

Usage: check-offline-firefox.py PATH_TO_SITE_DIR

The site directory is the one written under the base, for example
.scratch/offline/wiki. Requires Firefox and Selenium.
"""

import sys
from pathlib import Path

try:
    from selenium import webdriver
    from selenium.webdriver.common.by import By
    from selenium.webdriver.common.keys import Keys
    from selenium.webdriver.firefox.options import Options
    from selenium.webdriver.support.ui import WebDriverWait
except ImportError as exc:
    raise SystemExit(
        "Selenium is required: python3 -m pip install selenium"
    ) from exc


def must(condition, message):
    if not condition:
        raise AssertionError(message)


def wait_for(driver, css, timeout=20):
    return WebDriverWait(driver, timeout).until(
        lambda d: d.find_elements(By.CSS_SELECTOR, css)
    )


def non_file_requests(driver):
    return driver.execute_script(
        """
        return performance.getEntriesByType('resource')
          .map((entry) => entry.name)
          .filter((url) => !url.startsWith('file:') && !url.startsWith('data:'));
        """
    )


def check_page(driver, url, contain=None):
    driver.get(url)
    if contain:
        wait_for(driver, contain)
    must(not non_file_requests(driver), f"non-file request from {url}")


def open_search(driver):
    driver.find_element(By.ID, "searchBtn").click()
    field = WebDriverWait(driver, 5).until(
        lambda d: d.find_element(By.ID, "searchInput")
    )
    field.clear()
    field.click()
    return field


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: check-offline-firefox.py PATH_TO_SITE_DIR")
    root = Path(sys.argv[1]).resolve()
    base = root.as_uri() + "/"

    options = Options()
    options.add_argument("-headless")
    driver = webdriver.Firefox(options=options)
    driver.set_window_size(1440, 1000)
    try:
        driver.get(base + "index.html")
        actual_base = driver.execute_script("return String(document.baseURI);")
        must(actual_base == base, f"wrong document base: {actual_base!r}")
        styles = driver.execute_script("return document.styleSheets.length;")
        must(styles >= 3, "styles did not load")
        offline = driver.execute_script("return window.__WIKI_OFFLINE;")
        must(offline is True, "offline flag missing")
        must(not non_file_requests(driver), "index made a non-file request")

        field = open_search(driver)
        field.send_keys("offline")
        results = wait_for(driver, ".search-result-item")
        must(results, "search returned no results")
        content = driver.execute_script("return typeof window.__WIKI_SEARCH_CONTENT;")
        must(content == "object", "the content catalogue did not load")
        field.send_keys(Keys.ARROW_DOWN, Keys.ENTER)
        WebDriverWait(driver, 5).until(lambda d: d.current_url != base + "index.html")

        driver.get(base + "index.html")
        field = open_search(driver)
        field.send_keys("#offline")
        wait_for(driver, ".search-result-item")
        must(
            any(
                "/tags/offline.html" in a.get_attribute("href")
                for a in driver.find_elements(By.CSS_SELECTOR, ".search-result-item")
            ),
            "tag search did not reach the tag page",
        )

        check_page(driver, base + "design/rendering.html", ".mermaid-frame svg")
        frames = driver.execute_script(
            "return document.querySelectorAll('.mermaid-frame svg').length;"
        )
        must(frames > 0, "Mermaid did not render")

        check_page(driver, base + "graph.html", ".graph-node-group")
        nodes = driver.execute_script(
            "return document.querySelectorAll('.graph-node-group').length;"
        )
        must(nodes > 0, "graph has no nodes")
        inline = driver.execute_script("return !!document.getElementById('graphData');")
        must(inline, "graph is not inline")

        redirect = root.parent / "index.html"
        start = redirect.as_uri()
        driver.get(start)
        WebDriverWait(driver, 5).until(lambda d: d.current_url != start)
        must(
            driver.current_url.endswith("index.html"),
            "root redirect did not land on an index page",
        )
    finally:
        driver.quit()
    print("Firefox offline smoke test passed")


if __name__ == "__main__":
    main()
