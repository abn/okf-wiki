package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderChartSite(t *testing.T, files map[string]string, vendor string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, path, body)
	}
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, VendorDir: vendor})
	if err != nil {
		return "", err
	}
	out := t.TempDir()
	return out, r.RenderAll(out)
}

func chartVendor(t *testing.T) string {
	t.Helper()
	v := t.TempDir()
	mustWrite(t, filepath.Join(v, "mermaid-bundle.min.mjs"), "export default {};\n")
	mustWrite(t, filepath.Join(v, "mermaid-bundle.min.js"), "window.__WIKI_MERMAID = {};\n")
	mustWrite(t, filepath.Join(v, "vega-bundle.min.js"), "window.__WIKI_VEGA = {};\n")
	return v
}

const chartPage = "---\ntitle: Home\n---\n# Home\n\n```vega-lite\n" +
	`{"description": "Pages per quarter </script>",
  "title": "Pages",
  "data": {"values": [{"q": "Q1", "pages": 230}, {"q": "Q2", "pages": 310}]},
  "mark": "bar",
  "encoding": {"x": {"field": "q"}, "y": {"field": "pages", "type": "quantitative"}}}` +
	"\n```\n"

// TestChartFenceRendersFigure pins what a vega-lite fence becomes: a figure
// with the spec as inert JSON, a caption, a data table, and the chart client
// loaded only on that page.
func TestChartFenceRendersFigure(t *testing.T) {
	out, err := renderChartSite(t, map[string]string{
		"index.md":       chartPage,
		"guide/index.md": "---\ntitle: Guide\n---\n# Guide\n\nNo chart here.\n",
	}, chartVendor(t))
	if err != nil {
		t.Fatal(err)
	}
	page := readRendered(t, filepath.Join(out, "wiki", "index.html"))
	for _, want := range []string{
		`<figure class="chart" data-chart="vega-lite" aria-label="Pages">`,
		`<script type="application/json" class="chart-spec">`,
		`"width":"container"`,
		`<figcaption class="chart-caption">Pages per quarter &lt;/script&gt;</figcaption>`,
		`<summary>Data for Pages</summary>`,
		`<th scope="col">q</th><th scope="col">pages</th>`,
		`<td>Q2</td><td>310</td>`,
		`src="/wiki/charts.js?v=`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("chart page is missing %q", want)
		}
	}
	// The spec's own text must not be able to close the script it sits in.
	spec := page[strings.Index(page, `class="chart-spec">`):]
	spec = spec[:strings.Index(spec, "</script>")]
	if strings.Contains(spec, "</") {
		t.Errorf("spec JSON carries an unescaped closing tag: %s", spec)
	}
	other := readRendered(t, filepath.Join(out, "wiki", "guide", "index.html"))
	if strings.Contains(other, "charts.js") {
		t.Error("a page with no chart loads the chart client")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "vendor", "vega-bundle.min.js")); err != nil {
		t.Error("the chart runtime was not copied")
	}
	for _, unused := range []string{"mermaid-bundle.min.mjs", "mermaid-bundle.min.js"} {
		if _, err := os.Stat(filepath.Join(out, "wiki", "vendor", unused)); err == nil {
			t.Errorf("a site with no diagram copied %s", unused)
		}
	}
}

// TestChartDataFileIsInlined pins that a data file named relative to the page
// travels inside the page, so the chart draws from file:// and under any base.
func TestChartDataFileIsInlined(t *testing.T) {
	out, err := renderChartSite(t, map[string]string{
		"index.md": "---\ntitle: Home\n---\n# Home\n",
		"guide/index.md": "---\ntitle: Guide\n---\n# Guide\n\n```vega-lite\n" +
			`{"data": {"url": "data/latency.csv"}, "mark": "line",
  "encoding": {"x": {"field": "week", "type": "quantitative"}, "y": {"field": "ms", "type": "quantitative"}}}` +
			"\n```\n",
		"guide/data/latency.csv": "week,ms\n1,114\n2,111\n",
	}, chartVendor(t))
	if err != nil {
		t.Fatal(err)
	}
	page := readRendered(t, filepath.Join(out, "wiki", "guide", "index.html"))
	for _, want := range []string{
		`"data":{"values":[{"week":1,"ms":114},{"week":2,"ms":111}]}`,
		`<caption>data/latency.csv</caption>`,
		`<td>2</td><td>111</td>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("chart page is missing %q", want)
		}
	}
	if strings.Contains(page, `"url"`) {
		t.Error("the data url survived into the page")
	}
}

// TestChartCSVIsTyped pins how a CSV column is typed at render time: numbers
// and booleans by column, an empty cell in a typed column as null, and any
// other column, dates included, as text. Parsing here keeps Vega's CSV reader,
// which needs eval, out of the browser.
func TestChartCSVIsTyped(t *testing.T) {
	out, err := renderChartSite(t, map[string]string{
		"index.md": "---\ntitle: Home\n---\n# Home\n\n```vega-lite\n" +
			`{"data": {"url": "d.tsv", "format": {"parse": {"day": "date"}}}, "mark": "point"}` +
			"\n```\n",
		"d.tsv": "day\tn\tok\tcode\n2026-01-01\t1.5\ttrue\t007\n2026-01-02\t\tfalse\tA1\n",
	}, chartVendor(t))
	if err != nil {
		t.Fatal(err)
	}
	page := readRendered(t, filepath.Join(out, "wiki", "index.html"))
	want := `"data":{"format":{"parse":{"day":"date"}},"values":[` +
		`{"day":"2026-01-01","n":1.5,"ok":true,"code":"007"},` +
		`{"day":"2026-01-02","n":null,"ok":false,"code":"A1"}]}`
	if !strings.Contains(page, want) {
		t.Errorf("typed rows missing; want %s", want)
	}
}

// TestChartDataSymlinkOutOfBundle pins that a data file reached through a
// symlink pointing out of the bundle is refused like a path that climbs out.
func TestChartDataSymlinkOutOfBundle(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.csv")
	mustWrite(t, outside, "a\n1\n")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\n```vega-lite\n"+
		`{"data": {"url": "s.csv"}, "mark": "bar"}`+"\n```\n")
	if err := os.Symlink(outside, filepath.Join(dir, "s.csv")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	_, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, VendorDir: chartVendor(t)})
	if err == nil || !strings.Contains(err.Error(), "outside the bundle") {
		t.Fatalf("got %v, want an outside-the-bundle error", err)
	}
}

// TestChartErrorsFailTheRender pins that a chart which cannot be built stops
// the render with the file and line, rather than shipping a broken figure.
func TestChartErrorsFailTheRender(t *testing.T) {
	cases := map[string]struct {
		spec string
		want string
	}{
		"not json":       {`{"mark": "bar",}`, "not valid JSON"},
		"missing file":   {`{"data": {"url": "nope.csv"}, "mark": "bar"}`, `data "nope.csv"`},
		"outside bundle": {`{"data": {"url": "../../secret.csv"}, "mark": "bar"}`, "outside the bundle"},
		"remote":         {`{"data": {"url": "https://example.com/d.csv"}, "mark": "bar"}`, "name a file in the bundle"},
		"unknown format": {`{"data": {"url": "d.dat"}, "mark": "bar"}`, "set format.type"},
		"bad csv":        {`{"data": {"url": "bad.csv"}, "mark": "bar"}`, "not valid CSV"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := renderChartSite(t, map[string]string{
				"index.md": "---\ntitle: Home\n---\n# Home\n\nText.\n\n```vega-lite\n" + tc.spec + "\n```\n",
				"d.dat":    "x\n",
				"bad.csv":  "a,b\n\"unterminated\n",
			}, chartVendor(t))
			if err == nil {
				t.Fatal("render succeeded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "index.md:8: vega-lite chart") {
				t.Errorf("error %q does not name the file and line", err)
			}
		})
	}
}

// TestChartNeedsVendoredRuntime pins that a chart site with a vendor directory
// but no chart bundle fails as a runtime problem, the way a diagram site does.
func TestChartNeedsVendoredRuntime(t *testing.T) {
	v := t.TempDir()
	mustWrite(t, filepath.Join(v, "mermaid-bundle.min.mjs"), "export default {};\n")
	_, err := renderChartSite(t, map[string]string{"index.md": chartPage}, v)
	if !errors.Is(err, errVendorRuntime) {
		t.Fatalf("got %v, want a vendor runtime error", err)
	}
}
