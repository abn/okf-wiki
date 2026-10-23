#!/usr/bin/env python3
"""Smoke test an --offline render in Firefox over file://.

Usage: check-offline-firefox.py PATH_TO_SITE_DIR

The site directory is the one written under the base, for example
.scratch/offline/wiki. Requires Firefox and Selenium.
"""

import json
import re
import sys
import time
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


def wait_layout(driver):
    WebDriverWait(driver, 20).until(
        lambda d: d.find_element(By.ID, "graphApp").get_attribute("data-layout")
        == "ready"
    )


def write_force_fixture(root):
    source = (root / "graph.html").read_text()
    nodes = [
        {
            "id": f"page:p{i}",
            "title": f"Page {i}",
            "url": f"./p{i}.html",
            "kind": "page",
            "section": "Stress",
            "sectionId": "stress",
        }
        for i in range(1200)
    ]
    edges = [
        {"from": f"page:p{i}", "to": f"page:p{i + 1}", "kind": "link"}
        for i in range(1199)
    ]
    data = json.dumps({"version": 1, "nodes": nodes, "edges": edges})
    pattern = re.compile(
        r'<script type="application/json" id="graphData">.*?</script>',
        re.S,
    )
    html, count = pattern.subn(
        lambda _: '<script type="application/json" id="graphData">' + data + '</script>',
        source,
        count=1,
    )
    must(count == 1, "the graph page has no inline catalogue to replace")
    path = root / "stable-layout-test.html"
    path.write_text(html)
    return path


def visible_label_tiers(driver):
    return driver.execute_script(
        """
        const counts = {};
        for (const label of document.querySelectorAll('.graph-label')) {
          const tier = label.getAttribute('data-tier');
          if (!tier) continue;
          counts[tier] = counts[tier] || 0;
          if (!label.classList.contains('is-colliding')) counts[tier] += 1;
        }
        return counts;
        """
    )


def write_semantic_fixture(root):
    source = (root / "graph.html").read_text()
    nodes = [
        {
            "id": f"page:p{i}",
            "title": f"Page {i}",
            "url": f"./p{i}.html",
            "kind": "page",
            "section": "Overview",
            "sectionId": "overview",
        }
        for i in range(400)
    ]
    edges = [
        {"from": f"page:p{i}", "to": f"page:p{i + 1}", "kind": "link"}
        for i in range(7)
    ]
    data = json.dumps({"version": 1, "nodes": nodes, "edges": edges})
    pattern = re.compile(
        r'<script type="application/json" id="graphData">.*?</script>',
        re.S,
    )
    html, count = pattern.subn(
        lambda _: '<script type="application/json" id="graphData">' + data + '</script>',
        source,
        count=1,
    )
    must(count == 1, "the graph page has no inline catalogue to replace")
    path = root / "semantic-zoom-test.html"
    path.write_text(html)
    return path


def write_cross_fixture(root):
    source = (root / "graph.html").read_text()
    data = json.dumps({
        "version": 1,
        "nodes": [
            {
                "id": "alpha:page:index",
                "title": "Alpha home",
                "url": "alpha/index.html",
                "kind": "page",
                "site": "Alpha",
                "siteKey": "alpha",
            },
            {
                "id": "beta:page:index",
                "title": "Beta home",
                "url": "beta/index.html",
                "kind": "page",
                "site": "Beta",
                "siteKey": "beta",
            },
        ],
        "edges": [
            {
                "from": "alpha:page:index",
                "to": "beta:page:index",
                "kind": "link",
                "text": "Beta home",
            }
        ],
    })
    pattern = re.compile(
        r'<script type="application/json" id="graphData">.*?</script>',
        re.S,
    )
    html, count = pattern.subn(
        lambda _: '<script type="application/json" id="graphData">' + data + '</script>',
        source,
        count=1,
    )
    must(count == 1, "the graph page has no inline catalogue to replace")
    path = root / "cross-wiki-test.html"
    path.write_text(html)
    return path


def label_tier_totals(driver):
    return driver.execute_script(
        """
        const counts = {};
        for (const label of document.querySelectorAll('.graph-label')) {
          const tier = label.getAttribute('data-tier');
          if (!tier) continue;
          counts[tier] = (counts[tier] || 0) + 1;
        }
        return counts;
        """
    )


def graph_positions(driver):
    return driver.execute_script(
        """
        return Object.fromEntries(
          Array.from(document.querySelectorAll('.graph-node-group'))
            .map((group) => [group.getAttribute('data-id'), group.getAttribute('transform')])
        );
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

        check_page(driver, base + "usage/graph.html", ".connections-graph")
        driver.find_element(By.CSS_SELECTOR, ".connections-graph").click()
        WebDriverWait(driver, 10).until(lambda d: "focus=" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" in d.find_element(By.ID, "graphStatus").text
        )
        must(
            driver.find_elements(By.CSS_SELECTOR, ".graph-node-group.is-focus-far"),
            "the local graph did not fade nodes outside the neighbourhood",
        )
        must(
            "10 nodes" in driver.find_element(By.ID, "graphListStatus").text,
            "the list did not follow the focus",
        )
        must(
            len(driver.find_elements(By.CSS_SELECTOR, "#graphListNodes .graph-list-node"))
            == 10,
            "the list did not narrow to the focus set",
        )
        must(
            len(
                driver.find_elements(
                    By.CSS_SELECTOR, '#graphListNodes .graph-list-node[tabindex="0"]'
                )
            )
            == 1,
            "the list has more than one tab stop",
        )
        driver.find_element(By.ID, "graphFit").click()
        time.sleep(0.3)
        must(
            "Focused on" in driver.find_element(By.ID, "graphStatus").text,
            "Fit view cleared the focus",
        )
        driver.find_element(By.ID, "graphClearFocus").click()
        WebDriverWait(driver, 10).until(lambda d: "focus=" not in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" not in d.find_element(By.ID, "graphStatus").text
        )
        must(
            not driver.find_element(By.ID, "graphClearFocus").is_displayed(),
            "Show all stayed visible after clearing focus",
        )
        must(
            driver.execute_script("return document.activeElement.tagName;")
            .lower()
            != "body",
            "hiding Show all dropped keyboard focus to the document",
        )
        driver.back()
        WebDriverWait(driver, 10).until(lambda d: "focus=" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" in d.find_element(By.ID, "graphStatus").text
        )
        driver.forward()
        WebDriverWait(driver, 10).until(lambda d: "focus=" not in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" not in d.find_element(By.ID, "graphStatus").text
        )

        check_page(
            driver,
            base + "graph.html?kind=page&q=graph&tag=usage&tag=graph",
            ".graph-node-group",
        )
        must(
            driver.execute_script("return document.getElementById('graphKind').value;")
            == "page",
            "the kind did not come from the URL",
        )
        must(
            driver.execute_script("return document.getElementById('graphFilter').value;")
            == "graph",
            "the text filter did not come from the URL",
        )
        must(
            "2 selected"
            in driver.find_element(By.ID, "graphTagsSummary").text,
            "the tag selection did not come from the URL",
        )
        selected_tags = driver.execute_script(
            "return Array.from(document.querySelectorAll('#graphTagsMenu input:checked'))"
            ".map((box) => box.value).sort();"
        )
        must(selected_tags == ["graph", "usage"], "wrong tags selected from the URL")
        must(
            len(driver.find_elements(By.CSS_SELECTOR, "#graphListNodes .graph-list-node"))
            == 1,
            "the list did not follow the filtered graph",
        )
        must(
            "1 node" in driver.find_element(By.ID, "graphListStatus").text,
            "the list status did not follow the filtered graph",
        )

        driver.get(base + "graph.html")
        wait_for(driver, ".graph-node-group")
        driver.find_element(By.ID, "graphTagsSummary").click()
        wait_for(driver, "#graphTagsMenu input[value='usage']")
        driver.find_element(By.CSS_SELECTOR, "#graphTagsMenu input[value='usage']").click()
        WebDriverWait(driver, 10).until(lambda d: "tag=usage" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "#usage" in d.find_element(By.ID, "graphTagsSummary").text
        )
        driver.back()
        WebDriverWait(driver, 10).until(lambda d: "tag=usage" not in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Tags: any" in d.find_element(By.ID, "graphTagsSummary").text
        )
        driver.forward()
        WebDriverWait(driver, 10).until(lambda d: "tag=usage" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "#usage" in d.find_element(By.ID, "graphTagsSummary").text
        )
        driver.execute_script(
            "var k=document.getElementById('graphKind');"
            "k.value='page';k.dispatchEvent(new Event('change',{bubbles:true}));"
        )
        WebDriverWait(driver, 10).until(lambda d: "kind=page" in d.current_url)
        driver.back()
        WebDriverWait(driver, 10).until(lambda d: "kind=page" not in d.current_url)
        must(
            driver.execute_script("return document.getElementById('graphKind').value;")
            == "all",
            "the kind did not restore on back",
        )
        driver.forward()
        WebDriverWait(driver, 10).until(lambda d: "kind=page" in d.current_url)
        must(
            driver.execute_script("return document.getElementById('graphKind').value;")
            == "page",
            "the kind did not restore on forward",
        )

        driver.set_window_size(768, 1000)
        check_page(driver, base + "graph.html", ".graph-node-group")
        driver.find_element(By.ID, "graphTagsSummary").click()
        wait_for(driver, "#graphTagsMenu input")
        menu_right = driver.execute_script(
            "return document.querySelector('.graph-tags-menu').getBoundingClientRect().right;"
        )
        viewport = driver.execute_script(
            "return document.documentElement.clientWidth;"
        )
        must(menu_right <= viewport + 1, "the tag menu overflowed the tablet viewport")
        driver.set_window_size(1440, 1000)

        check_page(driver, base + "graph.html", ".graph-list-node")
        wait_layout(driver)
        must(
            driver.find_element(By.ID, "graphSections").is_selected(),
            "Sections is not on by default",
        )
        must(
            driver.find_element(By.ID, "graphTagHubs").is_selected(),
            "Tag hubs is not on by default",
        )
        tiers = visible_label_tiers(driver)
        must(tiers.get("1", 0) > 0, "no hub label is visible at the overview")
        must(
            tiers.get("2", 0) == 0,
            "ordinary page labels are visible before the reader zooms in",
        )
        must(
            "Labels show hubs" in driver.find_element(By.ID, "graphStatus").text,
            "the overview does not explain that only hubs are labelled",
        )
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: not d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )
        driver.execute_script(
            """
            const svg = document.getElementById('graphSvg');
            const canvas = svg.getBoundingClientRect();
            const target = Array.from(document.querySelectorAll('.graph-node-group')).find((group) => {
              const label = group.querySelector('.graph-label');
              if (!label || label.getAttribute('data-tier') !== '2') return false;
              const rect = group.getBoundingClientRect();
              return rect.left >= canvas.left && rect.right <= canvas.right &&
                rect.top >= canvas.top && rect.bottom <= canvas.bottom;
            });
            const rect = target ? target.getBoundingClientRect() : canvas;
            svg.dispatchEvent(new WheelEvent('wheel', {
              deltaY: -600,
              clientX: rect.left + rect.width / 2,
              clientY: rect.top + rect.height / 2,
              bubbles: true,
              cancelable: true,
            }));
            """
        )
        time.sleep(0.9)
        tiers = visible_label_tiers(driver)
        must(
            tiers.get("2", 0) > 0,
            "page labels did not appear after zooming in",
        )
        driver.execute_script(
            """
            const svg = document.getElementById('graphSvg');
            const rect = svg.getBoundingClientRect();
            svg.dispatchEvent(new WheelEvent('wheel', {
              deltaY: 600,
              clientX: rect.x + rect.width / 2,
              clientY: rect.y + rect.height / 2,
              bubbles: true,
              cancelable: true,
            }));
            """
        )
        time.sleep(0.4)
        tiers = visible_label_tiers(driver)
        must(
            tiers.get("2", 0) == 0,
            "page labels stayed visible after zooming out",
        )
        must(
            "Labels show hubs" in driver.find_element(By.ID, "graphStatus").text,
            "the overview hint did not return after zooming out",
        )
        driver.refresh()
        wait_for(driver, ".graph-section-label")
        wait_layout(driver)

        section_labels = driver.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        must(section_labels, "the section grid has no labels")
        must(
            driver.find_elements(By.CSS_SELECTOR, ".graph-section-box"),
            "the section grid has no boxes",
        )
        canvas_rect = driver.find_element(By.ID, "graphSvg").rect
        label_rects = [label.rect for label in section_labels]
        for rect in label_rects:
            must(
                rect["x"] >= canvas_rect["x"] - 1
                and rect["y"] >= canvas_rect["y"] - 1
                and rect["x"] + rect["width"] <= canvas_rect["x"] + canvas_rect["width"] + 1
                and rect["y"] + rect["height"] <= canvas_rect["y"] + canvas_rect["height"] + 1,
                "a section label was clipped by the canvas",
            )
        for i, first_rect in enumerate(label_rects):
            for second_rect in label_rects[i + 1:]:
                overlap_x = min(first_rect["x"] + first_rect["width"],
                                second_rect["x"] + second_rect["width"]) - max(first_rect["x"], second_rect["x"])
                overlap_y = min(first_rect["y"] + first_rect["height"],
                                second_rect["y"] + second_rect["height"]) - max(first_rect["y"], second_rect["y"])
                must(
                    overlap_x <= 1 or overlap_y <= 1,
                    "two section labels overlap",
                )
        for box in driver.find_elements(By.CSS_SELECTOR, ".graph-section-box"):
            rect = box.rect
            must(
                rect["x"] >= canvas_rect["x"] - 1
                and rect["y"] >= canvas_rect["y"] - 1
                and rect["x"] + rect["width"] <= canvas_rect["x"] + canvas_rect["width"] + 1
                and rect["y"] + rect["height"] <= canvas_rect["y"] + canvas_rect["height"] + 1,
                "a section box was clipped by the canvas",
            )
        node_label_rects = [
            label.rect for label in driver.find_elements(By.CSS_SELECTOR, ".graph-label")
            if label.is_displayed()
        ]
        for section_rect in label_rects:
            for node_rect in node_label_rects:
                overlap_x = min(section_rect["x"] + section_rect["width"],
                                node_rect["x"] + node_rect["width"]) - max(section_rect["x"], node_rect["x"])
                overlap_y = min(section_rect["y"] + section_rect["height"],
                                node_rect["y"] + node_rect["height"]) - max(section_rect["y"], node_rect["y"])
                must(
                    overlap_x <= 1 or overlap_y <= 1,
                    "a section label overlaps a node label",
                )
        baseline_nodes = len(driver.find_elements(By.CSS_SELECTOR, ".graph-node-group"))

        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: not d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )
        first_positions = graph_positions(driver)
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: not d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )
        must(
            graph_positions(driver) == first_positions,
            "the force layout changed when it was run again",
        )
        driver.refresh()
        wait_for(driver, ".graph-section-label")
        wait_layout(driver)
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: not d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )
        must(
            graph_positions(driver) == first_positions,
            "the force layout changed after a reload",
        )
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: d.find_elements(By.CSS_SELECTOR, ".graph-section-label")
        )

        driver.find_element(By.ID, "graphTagHubs").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: len(d.find_elements(By.CSS_SELECTOR, ".graph-node-group"))
            < baseline_nodes
        )
        collapsed_nodes = len(driver.find_elements(By.CSS_SELECTOR, ".graph-node-group"))
        must(
            not driver.find_elements(By.CSS_SELECTOR, ".graph-node-group.tag"),
            "tag hubs stayed visible after collapsing them",
        )
        must(
            driver.find_element(By.ID, "graphKind")
            .find_element(By.CSS_SELECTOR, 'option[value="tag"]')
            .get_attribute("disabled")
            is not None,
            "Show Tags stayed enabled with no tag hubs",
        )
        must(
            (str(collapsed_nodes) + " nodes")
            in driver.find_element(By.ID, "graphListStatus").text,
            "the list did not follow the collapsed tag hubs",
        )
        driver.find_element(By.ID, "graphTagHubs").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: len(d.find_elements(By.CSS_SELECTOR, ".graph-node-group"))
            == baseline_nodes
        )
        must(
            any(
                t.text == "Tag hubs"
                for t in driver.find_elements(By.CSS_SELECTOR, ".graph-section-label")
            ),
            "the restored tag hubs have no Tags cluster",
        )

        must(
            len(driver.find_elements(By.CSS_SELECTOR, "#graphListNodes .graph-list-node")) > 0,
            "the list has no nodes",
        )
        must(
            len(driver.find_elements(By.CSS_SELECTOR, "#graphListEdges .graph-list-edge")) > 0,
            "the list has no edges",
        )
        first = driver.find_element(By.CSS_SELECTOR, "#graphListNodes .graph-list-node")
        first_id = first.get_attribute("data-id")
        driver.execute_script("arguments[0].focus();", first)
        first.send_keys(Keys.ARROW_DOWN)
        must(
            driver.execute_script(
                "return document.activeElement.getAttribute('data-id');"
            )
            != first_id,
            "the list arrow keys did not move to the next node",
        )
        must(
            len(
                driver.find_elements(
                    By.CSS_SELECTOR, '#graphListNodes .graph-list-node[tabindex="0"]'
                )
            )
            == 1,
            "the list roving tab stop drifted",
        )
        driver.find_element(
            By.CSS_SELECTOR,
            '#graphListNodes button[data-id="page:usage/graph"]',
        ).click()
        WebDriverWait(driver, 10).until(lambda d: "focus=" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: d.find_element(By.ID, "graphDetails").is_displayed()
        )
        must(
            driver.find_element(By.CSS_SELECTOR, "#graphDetails h2").text == "Graph view",
            "selecting a list node did not open its details",
        )
        must(
            driver.find_element(
                By.CSS_SELECTOR, '#graphListNodes [aria-current="true"]'
            ).get_attribute("data-id")
            == "page:usage/graph",
            "the selected list node is not marked current",
        )
        must(
            driver.execute_script(
                "return document.activeElement.getAttribute('data-id');"
            )
            == "page:usage/graph",
            "selecting a list node moved keyboard focus away from it",
        )

        check_page(driver, base + "graph.html?q=graph", ".graph-list-node")
        wait_layout(driver)
        driver.find_element(By.CSS_SELECTOR, "#graphListNodes .graph-list-node").click()
        driver.find_element(By.ID, "graphSections").click()
        wait_layout(driver)
        WebDriverWait(driver, 5).until(
            lambda d: len(d.find_elements(By.CSS_SELECTOR, '#graphSvg circle[tabindex="0"]'))
            == 1
        )
        must(
            not driver.execute_script(
                "return document.querySelector('#graphSvg circle[tabindex=\"0\"]')"
                ".parentElement.classList.contains('is-dim');"
            ),
            "the graph tab stop went stale after a relayout",
        )

        check_page(driver, base + "graph.html?tag=no-such-tag", ".graph-node-group")
        WebDriverWait(driver, 5).until(
            lambda d: "Not in this graph" in d.find_element(By.ID, "graphStatus").text
        )
        must(
            driver.find_elements(By.CSS_SELECTOR, "#graphListNodes .graph-list-empty"),
            "the list has no empty state",
        )

        check_page(
            driver,
            base + "graph.html?focus=page%3Ausage%2Fgraph&depth=1",
            ".graph-clear",
        )
        driver.find_element(By.CSS_SELECTOR, ".graph-clear").click()
        WebDriverWait(driver, 10).until(lambda d: "focus=" not in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" not in d.find_element(By.ID, "graphStatus").text
        )
        must(
            not driver.find_element(By.ID, "graphClearFocus").is_displayed(),
            "Clear selection left Show all visible",
        )
        driver.back()
        WebDriverWait(driver, 10).until(lambda d: "focus=" in d.current_url)
        WebDriverWait(driver, 5).until(
            lambda d: "Focused on" in d.find_element(By.ID, "graphStatus").text
        )
        driver.forward()
        WebDriverWait(driver, 10).until(lambda d: "focus=" not in d.current_url)

        check_page(driver, base + "graph.html?focus=page%3Amissing", ".graph-node-group")
        WebDriverWait(driver, 5).until(
            lambda d: "Focus node not found" in d.find_element(By.ID, "graphStatus").text
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

        force_page = write_force_fixture(root)
        try:
            driver.get(force_page.as_uri())
            wait_for(driver, ".graph-node-group")
            wait_layout(driver)
            driver.find_element(By.ID, "graphSections").click()
            wait_layout(driver)
            force_first = graph_positions(driver)
            driver.find_element(By.ID, "graphSections").click()
            wait_layout(driver)
            driver.find_element(By.ID, "graphSections").click()
            wait_layout(driver)
            must(
                graph_positions(driver) == force_first,
                "the synthetic force layout changed when it was run again",
            )
            nudges = int(
                driver.find_element(By.ID, "graphApp").get_attribute("data-layout-nudges")
                or 0
            )
            must(nudges > 0, "the synthetic force layout produced no collision nudge")
        finally:
            force_page.unlink(missing_ok=True)

        semantic_page = write_semantic_fixture(root)
        try:
            driver.get(semantic_page.as_uri())
            wait_for(driver, ".graph-node-group")
            wait_layout(driver)
            totals = label_tier_totals(driver)
            must(
                totals.get("1", 0) <= 100,
                "the hub tier is wider than a quarter of the pages",
            )
            must(
                totals.get("2", 0) > 0,
                "the sparse fixture has no ordinary page tier",
            )
            driver.execute_script(
                """
                const svg = document.getElementById('graphSvg');
                const rect = svg.getBoundingClientRect();
                svg.dispatchEvent(new WheelEvent('wheel', {
                  deltaY: 600,
                  clientX: rect.x + rect.width / 2,
                  clientY: rect.y + rect.height / 2,
                  bubbles: true,
                  cancelable: true,
                }));
                """
            )
            time.sleep(0.4)
            tiers = visible_label_tiers(driver)
            must(
                tiers.get("2", 0) == 0,
                "ordinary page labels are visible at the sparse overview",
            )
            must(
                "Labels show hubs" in driver.find_element(By.ID, "graphStatus").text,
                "the sparse overview does not explain the hub tier",
            )
        finally:
            semantic_page.unlink(missing_ok=True)

        cross_page = write_cross_fixture(root)
        try:
            driver.get(cross_page.as_uri())
            wait_for(driver, ".graph-node-group")
            wait_layout(driver)
            must(
                driver.find_elements(By.CSS_SELECTOR, ".graph-edge.cross-site"),
                "the cross-wiki edge is not styled as crossing wikis",
            )
            must(
                driver.find_element(By.CSS_SELECTOR, ".graph-list-edge-kind").text
                == "cross-wiki",
                "the edge list does not name the cross-wiki edge",
            )
        finally:
            cross_page.unlink(missing_ok=True)

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
