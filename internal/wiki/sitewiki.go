package wiki

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
)

// Section is a top-level content directory.
type Section struct {
	ID    string // directory name ("" for the bundle root)
	Title string // display title
	Pages []Page
}

// Page is one rendered document.
type Page struct {
	Section      string
	Slug         string // relative path without extension, e.g. hosts/example
	Title        string
	Type         string
	Description  string
	Status       string
	Tags         []string
	GeneratedAt  string
	Body         string // rendered HTML, H1 still present; RenderPage strips it
	TOC          []TOCEntry
	searchChunks []rawSearchChunk
}

// TOCEntry is a heading in the page body.
type TOCEntry struct {
	ID    string
	Level int
	Text  string
}

// Renderer renders an OKF bundle.
type Renderer struct {
	md           goldmark.Markdown
	cfg          Config
	theme        *Theme
	sections     []Section
	specs        []sectionSpec
	generatedAt  string
	assetVersion string
}

// New builds a renderer for the bundle described by cfg.
func New(cfg Config) (*Renderer, error) {
	if cfg.Content == "" {
		return nil, fmt.Errorf("content directory is required")
	}
	if err := cfg.Brand.validate(); err != nil {
		return nil, err
	}
	theme, err := ResolveTheme(cfg.ThemeDir)
	if err != nil {
		return nil, err
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(newCustomRenderer(), 500)),
		),
	)
	r := &Renderer{
		md:           md,
		cfg:          cfg,
		theme:        theme,
		generatedAt:  time.Now().UTC().Format("2006-01-02"),
		assetVersion: theme.AssetVersion(),
	}
	if err := r.load(cfg.Content); err != nil {
		return nil, err
	}
	return r, nil
}

// Version returns the OKF specification version, not the binary's own version.
func (r *Renderer) Version() string { return "OKF v0.2" }

// GeneratedAt returns the render date, formatted 2006-01-02 in UTC.
func (r *Renderer) GeneratedAt() string { return r.generatedAt }

func (r *Renderer) AssetVersion() string { return r.assetVersion }

func (r *Renderer) Brand() Brand { return r.cfg.Brand }

func (r *Renderer) Theme() *Theme { return r.theme }

// Sections returns the sections in render order, with empty sections omitted.
func (r *Renderer) Sections() []Section { return r.sections }

// RenderAll renders every page into <out>/<base>/ and writes the resolved theme.
func (r *Renderer) RenderAll(out string) error {
	site, err := r.cfg.SiteDir(out)
	if err != nil {
		return err
	}
	// Start clean so removed pages, renamed sections, or an outdated vendor
	// bundle never linger in the output.
	if err := os.RemoveAll(site); err != nil {
		return err
	}
	if err := os.MkdirAll(site, 0o755); err != nil {
		return err
	}
	for _, s := range r.sections {
		for _, p := range s.Pages {
			rel := p.Slug + ".html"
			if p.Slug == s.ID {
				rel = s.ID + "/index.html"
			}
			dst := filepath.Join(site, filepath.Dir(rel))
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(site, rel), []byte(r.RenderPage(p)), 0o644); err != nil {
				return err
			}
		}
	}

	if err := r.theme.WriteTo(site); err != nil {
		return err
	}
	if err := copyBundleAssets(r.cfg.Content, site); err != nil {
		return err
	}
	if err := copyVendor(r.cfg.VendorDir, filepath.Join(site, "vendor")); err != nil {
		return err
	}

	index, err := r.SearchIndexJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "search-index.json"), index, 0o644); err != nil {
		return err
	}

	// theme.json in the output records which layer produced the site.
	manifest, err := r.theme.describe(r.cfg.base())
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "theme.json"), manifest, 0o644); err != nil {
		return err
	}

	// A root-level redirect, so serving the output directory lands on the wiki
	// whatever the base is.
	//
	// Skipped when the site is already the output root, which is what base "/"
	// produces: the wiki's own index.html sits at this path, so writing the
	// redirect would replace the home page with a self-redirect.
	base := r.cfg.base()
	if filepath.Clean(site) == filepath.Clean(out) {
		return nil
	}
	redirect := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">` +
		`<meta http-equiv="refresh" content="0; url=` + base + `"><title>` +
		htmlEscape(r.cfg.Brand.Title) + `</title></head>` +
		`<body><a href="` + base + `">` + htmlEscape(r.cfg.Brand.Title) + `</a></body></html>`
	return os.WriteFile(filepath.Join(out, "index.html"), []byte(redirect), 0o644)
}

// load builds sections from the directories directly under root, plus a
// top-level section for the root Markdown files. It is one level deep, so a
// page in a nested directory is dropped without a word.
func (r *Renderer) load(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var present []string
	for _, d := range entries {
		if d.IsDir() {
			present = append(present, d.Name())
		}
	}
	r.specs = orderedSectionIDs(parseSections(r.cfg.Brand.Sections), present)

	for _, spec := range r.specs {
		if spec.ID == "" {
			if err := r.loadTopLevel(root, spec.Title); err != nil {
				return err
			}
			continue
		}
		dir := filepath.Join(root, spec.ID)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		sec, err := r.loadSection(dir, spec.ID, spec.Title)
		if err != nil {
			return err
		}
		if len(sec.Pages) > 0 {
			r.sections = append(r.sections, sec)
		}
	}
	return nil
}

func (r *Renderer) loadTopLevel(root, title string) error {
	sec := Section{ID: "", Title: title}
	files, _ := filepath.Glob(filepath.Join(root, "*.md"))
	sort.Strings(files)
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".md")
		p, err := r.parsePage("", base, f)
		if err != nil {
			return err
		}
		sec.Pages = append(sec.Pages, p)
	}
	if len(sec.Pages) > 0 {
		sort.Slice(sec.Pages, func(i, j int) bool {
			if sec.Pages[i].Slug == "index" {
				return true
			}
			if sec.Pages[j].Slug == "index" {
				return false
			}
			return sec.Pages[i].Slug < sec.Pages[j].Slug
		})
		r.sections = append(r.sections, sec)
	}
	return nil
}

func (r *Renderer) loadSection(dir, id, title string) (Section, error) {
	sec := Section{ID: id, Title: title}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Strings(files)
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".md")
		p, err := r.parsePage(id, id+"/"+base, f)
		if err != nil {
			return sec, err
		}
		sec.Pages = append(sec.Pages, p)
	}
	sort.Slice(sec.Pages, func(i, j int) bool {
		if sec.Pages[i].Slug == id+"/index" {
			return true
		}
		if sec.Pages[j].Slug == id+"/index" {
			return false
		}
		return sec.Pages[i].Title < sec.Pages[j].Title
	})
	return sec, nil
}

// frontmatter holds the OKF v0.2 fields the renderer consumes. An unparsable
// block is not an error: the page falls back to its first heading for a title.
type frontmatter struct {
	Title       string   `yaml:"title"`
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Status      string   `yaml:"status"`
	Tags        []string `yaml:"tags"`
	Generated   struct {
		By string `yaml:"by"`
		At string `yaml:"at"`
	} `yaml:"generated"`
}

func (r *Renderer) parsePage(section, slug, file string) (Page, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Page{}, err
	}
	fm, body := splitFrontmatter(data)

	p := Page{
		Section:     section,
		Slug:        slug,
		Type:        fm.Type,
		Description: fm.Description,
		Status:      fm.Status,
		Tags:        fm.Tags,
		GeneratedAt: fm.Generated.At,
		Title:       fm.Title,
	}

	doc := r.md.Parser().Parse(text.NewReader(body))
	src := body
	usedIDs := map[string]bool{}
	var h1 string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nn := n.(type) {
		case *ast.Link:
			nn.Destination = []byte(r.linkPath(p.Slug, string(nn.Destination)))
		case *ast.Image:
			// An image destination is a file in the bundle, and is copied into
			// the site. Rewriting it the way a link is rewritten is what stops
			// a relative src from pointing at a path that was never written.
			nn.Destination = []byte(r.linkPath(p.Slug, string(nn.Destination)))
		case *ast.Blockquote:
			detectAlert(nn, src)
		}
		if h := headingText(n, src); h != "" && p.Title == "" && isH1(n) {
			h1 = h
			p.Title = h
		}
		if isHeading(n) && !isH1(n) {
			h := n.(*ast.Heading)
			headingVal := headingText(n, src)
			id := uniqueID(slugify(headingVal), usedIDs)
			h.SetAttribute([]byte("id"), []byte(id))
			if h.Level <= 3 {
				p.TOC = append(p.TOC, TOCEntry{ID: id, Level: h.Level, Text: headingVal})
			}
		}
		return ast.WalkContinue, nil
	})

	if p.Title == "" {
		p.Title = h1
	}
	if p.Title == "" {
		p.Title = slug
	}
	p.searchChunks = extractSearchChunks(doc, src, p.Title)

	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, src, doc); err != nil {
		return Page{}, err
	}
	p.Body = buf.String()
	return p, nil
}

func splitFrontmatter(data []byte) (frontmatter, []byte) {
	var fm frontmatter
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return fm, data
	}
	end := bytes.Index(data[4:], []byte("\n---\n"))
	if end == -1 {
		return fm, data
	}
	_ = yaml.Unmarshal(data[4:4+end], &fm)
	return fm, data[4+end+5:]
}

// linkPath resolves a markdown link from a page slug into a wiki URL. Links that
// stay inside the bundle and end in .md map to rendered HTML pages. Anything
// else, in-bundle or not, maps to /repo/, which only resolves when a
// repository root was supplied; with none, a non-.md href should stay relative
// rather than become a dead /repo/ URL.
func (r *Renderer) linkPath(fromSlug, href string) string {
	if strings.Contains(href, "://") || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return href
	}
	parts := strings.SplitN(href, "#", 2)
	target := parts[0]
	frag := ""
	if len(parts) == 2 {
		frag = "#" + parts[1]
	}
	if target == "" {
		return href
	}
	dir := path.Dir(fromSlug)
	clean := path.Clean(path.Join(dir, target))

	// A target that climbs out of the bundle is not ours to publish. It can
	// only be reached through a repository mount, so without one the href is
	// left as written rather than pointed at a /repo/ URL nothing serves.
	if strings.HasPrefix(clean, "..") {
		if r.cfg.Repo == "" {
			return href
		}
		repoRel := strings.TrimPrefix(clean, "../")
		if repoRel == "" || strings.HasPrefix(repoRel, "..") {
			return href
		}
		return "/repo/" + repoRel + frag
	}
	if !strings.HasSuffix(clean, ".md") {
		// Every non-Markdown file in the bundle is copied into the site under
		// its own path, so an in-bundle asset resolves like a page does.
		return r.cfg.base() + clean + frag
	}
	return r.cfg.base() + strings.TrimSuffix(clean, ".md") + ".html" + frag
}

func isHeading(n ast.Node) bool {
	_, ok := n.(*ast.Heading)
	return ok
}

func isH1(n ast.Node) bool {
	h, ok := n.(*ast.Heading)
	return ok && h.Level == 1
}

func headingText(n ast.Node, src []byte) string {
	h, ok := n.(*ast.Heading)
	if !ok {
		return ""
	}
	var b strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		collectText(c, src, &b)
	}
	return strings.TrimSpace(b.String())
}

func collectText(n ast.Node, src []byte, b *strings.Builder) {
	switch nn := n.(type) {
	case *ast.Text:
		b.Write(nn.Segment.Value(src))
	case *ast.String:
		b.Write(nn.Value)
	case *ast.CodeSpan:
		for c := nn.FirstChild(); c != nil; c = c.NextSibling() {
			collectText(c, src, b)
		}
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collectText(c, src, b)
		}
	}
}

// uniqueID returns a heading id not yet present in taken, and records it.
//
// The obvious approach, counting repeats of one base slug, is not enough.
// "Setup", "Setup", "Setup" and "Setup 2" slugify to setup, setup, setup and
// setup-2, and the second repeat is already numbered setup-2, so the literal
// "Setup 2" heading collides with it. Every candidate has to be checked against
// the ids already emitted, including the suffixed ones, so the sequence is
// setup, setup-2, setup-3, setup-2-2.
func uniqueID(base string, taken map[string]bool) string {
	id := base
	if id == "" {
		// A heading of only punctuation or an emoji slugs to nothing.
		id = "section"
	}
	candidate := id
	for n := 2; taken[candidate]; n++ {
		candidate = fmt.Sprintf("%s-%d", id, n)
	}
	taken[candidate] = true
	return candidate
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	res := b.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}

// copyBundleAssets copies every non-Markdown file in the bundle into the site,
// keeping its path. Without this an image in a page is a link to a file the
// render never wrote, so the page ships with a broken image and no error.
//
// Dotfiles and dot-directories are skipped: a bundle directory is a real
// working tree as often as not, and nothing in a wiki should be able to publish
// a .git directory or a .env.
func copyBundleAssets(root, dest string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		return copyFile(p, filepath.Join(dest, rel))
	})
}

// copyVendor copies the mermaid bundle directory into the output, if present.
func copyVendor(dir, dest string) error {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == "node_modules" {
				continue
			}
			if err := copyTree(filepath.Join(dir, e.Name()), filepath.Join(dest, e.Name())); err != nil {
				return err
			}
			continue
		}
		if !isVendorArtifact(e.Name()) {
			continue
		}
		if err := copyFile(filepath.Join(dir, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// isVendorArtifact reports whether a vendor file is a runtime artifact, so
// build sources sharing the directory are not published.
func isVendorArtifact(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mjs", ".wasm", ".css", ".map":
		return true
	}
	return false
}

func copyTree(src, dest string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
