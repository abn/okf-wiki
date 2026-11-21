package wiki

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// chartLang is the fence info string that marks a Vega-Lite chart.
const chartLang = "vega-lite"

// chartAttr carries a chart's prepared HTML from the page walk, which knows the
// source file, to the node renderer, which does not.
const chartAttr = "data-okf-chart"

// chartMarker is what a rendered body contains when it has a chart, so a page
// can load the chart runtime only when it needs it.
const chartMarker = `data-chart="vega-lite"`

// Limits that keep a chart from turning a page into a data dump. The table is
// a readable fallback, not an export, and an inlined data file travels in the
// page, so both are bounded.
const (
	chartTableRows   = 100
	chartDataMaxSize = 2 << 20
)

// chartTable is one data source of a chart, shown as a table for a reader
// without JavaScript, a screen reader, or a printout taken before the chart
// was drawn.
type chartTable struct {
	label string
	cols  []string
	rows  [][]string
	total int
}

// prepareChart turns a vega-lite fence into the figure the page carries. Data
// files are read now and inlined, so a chart renders the same served, from a
// static host under a subpath, and from file://, where a browser will not
// fetch a sibling file. A file outside the bundle, a missing file, or a spec
// that is not JSON fails the render rather than shipping a broken chart.
func prepareChart(spec []byte, pageFile, root string) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(spec))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		return "", fmt.Errorf("spec is not valid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", errors.New("spec is not valid JSON: content after the top-level object")
	}
	top, ok := v.(*orderedObject)
	if !ok {
		return "", errors.New("spec must be a JSON object")
	}

	var tables []chartTable
	if err := walkChartData(top, filepath.Dir(pageFile), root, &tables); err != nil {
		return "", err
	}
	// A single view with no width fills the column, which is what a reader
	// expects of a figure in prose. A spec that sets a width keeps it.
	if _, has := top.get("width"); !has {
		if _, single := top.get("mark"); single {
			top.set("width", "container")
		}
	}

	out, err := json.Marshal(top)
	if err != nil {
		return "", err
	}

	title := chartTitle(top)
	desc, _ := top.get("description")
	descText, _ := desc.(string)

	var b strings.Builder
	b.WriteString(`<figure class="chart" ` + chartMarker)
	if title != "" {
		b.WriteString(` aria-label="` + template.HTMLEscapeString(title) + `"`)
	}
	b.WriteString(">\n")
	fmt.Fprintf(&b, "<div class=\"chart-view\" style=\"min-height:%dpx\"></div>\n", chartHeight(top))
	// json.Marshal escapes <, > and &, so the spec cannot close the script.
	b.WriteString(`<script type="application/json" class="chart-spec">`)
	b.Write(out)
	b.WriteString("</script>\n")
	if descText != "" {
		b.WriteString(`<figcaption class="chart-caption">` + template.HTMLEscapeString(descText) + "</figcaption>\n")
	}
	if len(tables) > 0 {
		writeChartTables(&b, tables, title)
	}
	b.WriteString("</figure>\n")
	return b.String(), nil
}

// chartTitle reads the spec title, which Vega-Lite allows as a string, a
// list of lines, or an object with a text field of either shape.
func chartTitle(top *orderedObject) string {
	t, ok := top.get("title")
	if !ok {
		return ""
	}
	if o, ok := t.(*orderedObject); ok {
		t, _ = o.get("text")
	}
	switch tt := t.(type) {
	case string:
		return tt
	case []any:
		var parts []string
		for _, p := range tt {
			if s, ok := p.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// chartHeight reserves the space a chart will take, so the page does not jump
// when it is drawn. Axes, a title and a legend sit outside the plot height.
func chartHeight(top *orderedObject) int {
	if h, ok := top.get("height"); ok {
		if n, ok := h.(json.Number); ok {
			if f, err := n.Float64(); err == nil && f > 0 {
				return int(f) + 90
			}
		}
	}
	return 320
}

// walkChartData inlines every data source that names a url, wherever it sits
// in the spec: top level, a layer, a concat, or a lookup. It collects a table
// for each source it can read as rows.
func walkChartData(v any, dir, root string, tables *[]chartTable) error {
	switch vv := v.(type) {
	case *orderedObject:
		for _, k := range vv.keys {
			child := vv.vals[k]
			if k == "data" {
				if d, ok := child.(*orderedObject); ok {
					if err := inlineChartData(d, dir, root, tables); err != nil {
						return err
					}
					continue
				}
			}
			if err := walkChartData(child, dir, root, tables); err != nil {
				return err
			}
		}
	case []any:
		for _, c := range vv {
			if err := walkChartData(c, dir, root, tables); err != nil {
				return err
			}
		}
	}
	return nil
}

func inlineChartData(d *orderedObject, dir, root string, tables *[]chartTable) error {
	label := ""
	if n, ok := d.get("name"); ok {
		label, _ = n.(string)
	}
	format, _ := d.get("format")
	fo, _ := format.(*orderedObject)
	ftype := ""
	if fo != nil {
		if t, ok := fo.get("type"); ok {
			ftype, _ = t.(string)
		}
	}

	var raw []byte
	ref := ""
	if u, ok := d.get("url"); ok {
		ref, _ = u.(string)
		b, err := readChartData(ref, dir, root)
		if err != nil {
			return err
		}
		raw = b
		if ftype == "" {
			switch strings.ToLower(filepath.Ext(ref)) {
			case ".csv":
				ftype = "csv"
			case ".tsv":
				ftype = "tsv"
			case ".json":
				ftype = "json"
			default:
				return fmt.Errorf("data %q: set format.type, since the extension does not say how to read it", ref)
			}
		}
		d.del("url")
		if label == "" {
			label = ref
		}
	} else if vals, ok := d.get("values"); ok {
		switch vv := vals.(type) {
		case []any:
			if t, ok := tableFromValues(vv); ok {
				t.label = label
				*tables = append(*tables, t)
			}
			return nil
		case string:
			raw = []byte(vv)
		default:
			return nil
		}
	} else {
		return nil
	}

	// Rows are parsed here rather than in the browser. Vega's CSV reader
	// builds each row with new Function, which a Content-Security-Policy
	// without 'unsafe-eval' refuses, and a parsed array also spares the
	// reader's device the work.
	recs, err := recordsFromText(raw, ftype, fo)
	if err != nil {
		name := ref
		if name == "" {
			name = "values"
		}
		return fmt.Errorf("data %q: %w", name, err)
	}
	if recs == nil {
		if fo == nil {
			fo = &orderedObject{}
			d.set("format", fo)
		}
		if ftype != "" {
			fo.set("type", ftype)
		}
		d.set("values", string(raw))
		return nil
	}
	d.set("values", recs)
	if fo != nil {
		fo.del("type")
		if len(fo.keys) == 0 {
			d.del("format")
		}
	}
	if t, ok := tableFromValues(recs); ok {
		t.label = label
		*tables = append(*tables, t)
	}
	return nil
}

// recordsFromText parses CSV, TSV, or a JSON array of records into an array
// of records. It returns nil with no error for data it leaves to Vega, such
// as TopoJSON or a JSON object read through format.property.
func recordsFromText(raw []byte, ftype string, fo *orderedObject) ([]any, error) {
	switch ftype {
	case "csv", "tsv":
		r := csv.NewReader(bytes.NewReader(raw))
		if ftype == "tsv" {
			r.Comma = '\t'
			r.LazyQuotes = true
		}
		r.FieldsPerRecord = -1
		rows, err := r.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("not valid %s: %w", strings.ToUpper(ftype), err)
		}
		if len(rows) == 0 {
			return []any{}, nil
		}
		return typedRecords(rows[0], rows[1:]), nil
	case "json":
		if fo != nil {
			if _, ok := fo.get("property"); ok {
				return nil, nil
			}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		v, err := decodeOrdered(dec)
		if err != nil {
			return nil, fmt.Errorf("not valid JSON: %w", err)
		}
		if arr, ok := v.([]any); ok {
			return arr, nil
		}
	}
	return nil, nil
}

// typedRecords types each column the way Vega's automatic parsing does for
// numbers and booleans: a column whose every filled cell is a number becomes
// numbers, one of only true and false becomes booleans, and an empty cell in
// such a column is null. Anything else, dates included, stays text, which a
// temporal encoding still reads.
func typedRecords(header []string, rows [][]string) []any {
	kind := make([]string, len(header))
	for c := range header {
		isNum, isBool, filled := true, true, false
		for _, row := range rows {
			if c >= len(row) || row[c] == "" {
				continue
			}
			filled = true
			if !jsonNumber.MatchString(row[c]) {
				isNum = false
			}
			if row[c] != "true" && row[c] != "false" {
				isBool = false
			}
		}
		switch {
		case filled && isNum:
			kind[c] = "number"
		case filled && isBool:
			kind[c] = "bool"
		}
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		o := &orderedObject{}
		for c, h := range header {
			cell := ""
			if c < len(row) {
				cell = row[c]
			}
			switch {
			case kind[c] == "":
				o.set(h, cell)
			case cell == "":
				o.set(h, nil)
			case kind[c] == "number":
				o.set(h, json.Number(cell))
			default:
				o.set(h, cell == "true")
			}
		}
		out = append(out, o)
	}
	return out
}

var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// readChartData reads a data file named relative to the page. Only a file
// inside the bundle is allowed: a remote URL would make the page depend on the
// network, and a path that climbs out of the bundle would publish a file that
// was never part of it.
func readChartData(ref, dir, root string) ([]byte, error) {
	if ref == "" {
		return nil, errors.New("data url is empty")
	}
	if pu, err := url.Parse(ref); err != nil || pu.Scheme != "" || pu.Host != "" || strings.HasPrefix(ref, "/") {
		return nil, fmt.Errorf("data %q: name a file in the bundle, relative to the page", ref)
	}
	p := filepath.Join(dir, filepath.FromSlash(ref))
	if rel, err := filepath.Rel(root, p); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("data %q: outside the bundle", ref)
	}
	if _, err := os.Stat(p); err != nil {
		return nil, fmt.Errorf("data %q: %w", ref, err)
	}
	// Both sides are resolved, so a symlink that points out of the bundle is
	// caught as well as a path that climbs out of it.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	realP, err := filepath.EvalSymlinks(p)
	if err != nil {
		return nil, fmt.Errorf("data %q: %w", ref, err)
	}
	rel, err := filepath.Rel(realRoot, realP)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("data %q: outside the bundle", ref)
	}
	info, err := os.Stat(realP)
	if err != nil {
		return nil, fmt.Errorf("data %q: %w", ref, err)
	}
	if info.Size() > chartDataMaxSize {
		return nil, fmt.Errorf("data %q: %d bytes is over the %d byte limit for data carried in a page", ref, info.Size(), chartDataMaxSize)
	}
	return os.ReadFile(realP)
}

// tableFromValues builds a table from an array of records. Columns follow the
// order fields first appear in, which is the order the author wrote them.
func tableFromValues(vals []any) (chartTable, bool) {
	var t chartTable
	seen := map[string]bool{}
	for _, v := range vals {
		o, ok := v.(*orderedObject)
		if !ok {
			return chartTable{}, false
		}
		for _, k := range o.keys {
			if !seen[k] {
				seen[k] = true
				t.cols = append(t.cols, k)
			}
		}
	}
	if len(t.cols) == 0 {
		return chartTable{}, false
	}
	t.total = len(vals)
	for _, v := range vals {
		if len(t.rows) == chartTableRows {
			break
		}
		o := v.(*orderedObject)
		row := make([]string, len(t.cols))
		for i, c := range t.cols {
			if cell, ok := o.get(c); ok {
				row[i] = cellText(cell)
			}
		}
		t.rows = append(t.rows, row)
	}
	return t, true
}

func cellText(v any) string {
	switch vv := v.(type) {
	case nil:
		return ""
	case string:
		return vv
	case json.Number:
		return vv.String()
	case bool:
		return strconv.FormatBool(vv)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func writeChartTables(b *strings.Builder, tables []chartTable, title string) {
	summary := "Data"
	if title != "" {
		summary = "Data for " + title
	}
	b.WriteString(`<details class="chart-data"><summary>` + template.HTMLEscapeString(summary) + "</summary>\n")
	for _, t := range tables {
		b.WriteString("<table>\n")
		if t.label != "" || len(t.rows) < t.total {
			caption := t.label
			if len(t.rows) < t.total {
				if caption != "" {
					caption += ": "
				}
				caption += fmt.Sprintf("first %d of %d rows", len(t.rows), t.total)
			}
			b.WriteString("<caption>" + template.HTMLEscapeString(caption) + "</caption>\n")
		}
		b.WriteString("<thead><tr>")
		for _, c := range t.cols {
			b.WriteString(`<th scope="col">` + template.HTMLEscapeString(c) + "</th>")
		}
		b.WriteString("</tr></thead>\n<tbody>\n")
		for _, row := range t.rows {
			b.WriteString("<tr>")
			for _, cell := range row {
				b.WriteString("<td>" + template.HTMLEscapeString(cell) + "</td>")
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</tbody></table>\n")
	}
	b.WriteString("</details>\n")
}

// orderedObject is a JSON object that keeps its key order, so a spec comes
// back out the way the author wrote it and a table's columns follow the data.
type orderedObject struct {
	keys []string
	vals map[string]any
}

func (o *orderedObject) get(k string) (any, bool) {
	if o.vals == nil {
		return nil, false
	}
	v, ok := o.vals[k]
	return v, ok
}

func (o *orderedObject) set(k string, v any) {
	if o.vals == nil {
		o.vals = map[string]any{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedObject) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// MarshalJSON implements json.Marshaler.
func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// decodeOrdered reads one JSON value, keeping object key order. Numbers come
// back as json.Number when the decoder is set to use them.
func decodeOrdered(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &orderedObject{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("object key %v is not a string", kt)
				}
				v, err := decodeOrdered(dec)
				if err != nil {
					return nil, err
				}
				o.set(k, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return o, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeOrdered(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	default:
		return tok, nil
	}
}
