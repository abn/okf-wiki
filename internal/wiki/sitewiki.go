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
	Section string
	Slug    string // relative path without extension, e.g. hosts/example
	// Synthetic is set on a page the renderer builds itself, such as a tag
	// listing. It has no Markdown source in the bundle, so it offers nothing to
	// download.
	Synthetic bool
	Title     string
	// Nav overrides the label the sidebar shows for this page. A body title can
	// be a full sentence while the rail needs a few words, so frontmatter sets
	// this without changing the page itself.
	Nav    string
	Kicker string
	// TitleHTML is trusted H1 markup, set only by synthetic pages the
	// renderer builds itself. Frontmatter titles stay plain strings.
	TitleHTML    string
	Type         string
	Description  string
	Status       string
	Tags         []string
	StaleAfter   string
	Sources      []Source
	GeneratedAt  string
	Body         string // rendered HTML, H1 still present; RenderPage strips it
	TOC          []TOCEntry
	searchChunks []rawSearchChunk
}

// TOCEntry is a level-two heading in the page body. Deeper headings are link
// targets but never navigation entries, so the level needs no recording.
type TOCEntry struct {
	ID   string
	Text string
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
	// skipped records wikis in a tree that could not be read, so one bad bundle
	// is reported and left out rather than failing the whole render. A tree
	// assembled from linked checkouts is exactly where a stale link lands, and
	// taking every wiki down for one is worse than publishing the rest.
	skipped []SkippedSite
	// nested marks a renderer for one wiki inside a multi-site tree. It shares
	// the tree's theme, and it does not write the root redirect, which would
	// otherwise point the whole tree at whichever wiki rendered last.
	nested bool
	// searchBase is the URL prefix the search index is fetched from. It is the
	// renderer's own base for a single bundle; for a wiki or a group of a
	// multi-site tree it is the tree base, where one aggregate index spans every
	// wiki. The page carries it as window.__WIKI_SEARCH.
	searchBase string
	// aggregated collects the labelled search entries of a tree render, in the
	// order the wikis were visited, so the whole tree gets one index at its base.
	aggregated []SearchEntry
	// treeBase, siteRel, siteTitle and siteTrail describe a page's place in a
	// multi-site tree. treeBase is the tree index's URL prefix, empty for a
	// single bundle, so the brand and the breadcrumb trail mean "the deployment
	// home" only when there is a tree. siteRel is the page's wiki or group path
	// under the tree root ("" for the root), siteTitle its display title, and
	// siteTrail the folder crumbs from the tree root down to it. groupIndex
	// marks a page that is a generated group index rather than a wiki page.
	treeBase   string
	siteRel    string
	siteTitle  string
	siteTrail  []treeCrumb
	groupIndex bool
	// landing marks a renderer for the deployment's introduction page. It has
	// no sections of its own, so the sidebar stays empty; the brand points at
	// the output root the page sits at rather than at the wiki base.
	landing bool
}

// SkippedSite is one wiki a tree render left out, with the reason.
type SkippedSite struct {
	// Rel is the wiki's path under the tree root.
	Rel string
	// Err is why it could not be read.
	Err error
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
	// In multi-site mode the content directory is a tree, not a bundle: the
	// tree is walked and each wiki loaded as it is rendered, so loading the
	// root as one bundle here would splice every wiki into one site.
	if !cfg.MultiSite {
		if err := r.load(cfg.Content); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Version returns the OKF specification version, not the binary's own version.
func (r *Renderer) Version() string { return "OKF v0.2" }

// GeneratedAt returns the render date, formatted 2006-01-02 in UTC.
func (r *Renderer) GeneratedAt() string { return r.generatedAt }

func (r *Renderer) AssetVersion() string { return r.assetVersion }

func (r *Renderer) Theme() *Theme { return r.theme }

// Sections returns the sections in render order, with empty sections omitted.
func (r *Renderer) Sections() []Section { return r.sections }

// RenderAll renders every page into <out>/<base>/ and writes the resolved theme.
//
// The site is built beside its destination and renamed into place, so a render
// never exposes a directory with files missing. Only a single bundle swaps its
// own directory; a multi-site tree swaps the output root through RenderSites.
func (r *Renderer) RenderAll(out string) error {
	site, err := r.cfg.SiteDir(out)
	if err != nil {
		return err
	}
	return swapDir(site, func(tmp string) error {
		return r.renderInto(out, tmp)
	})
}

// renderInto builds the site into site, which is a temporary directory the swap
// helper will rename. out is kept for the root redirect, which belongs to the
// output root rather than to the site directory.
func (r *Renderer) renderInto(out, site string) error {
	if err := os.MkdirAll(site, 0o755); err != nil {
		return err
	}
	occupied := map[string]bool{}
	for _, s := range r.sections {
		for _, p := range s.Pages {
			rel := p.Slug + ".html"
			if p.Slug == s.ID {
				rel = s.ID + "/index.html"
			}
			occupied[rel] = true
			dst := filepath.Join(site, filepath.Dir(rel))
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			// Render before writing, so a theme that fails against the data
			// leaves no half-written page behind.
			body, err := r.RenderPage(p)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(site, rel), []byte(body), 0o644); err != nil {
				return err
			}
		}
	}

	// Tag pages live under tags/, a namespace no section may already occupy.
	// Overwriting a real page there would trade it for a listing silently,
	// so a collision fails the render and names both claimants.
	listings := r.tagListings()
	for _, t := range listings {
		rel := filepath.Join("tags", t.Slug+".html")
		if occupied[rel] {
			return fmt.Errorf("tag %q collides with a page at %s", t.Display, rel)
		}
		occupied[rel] = true
		if err := os.MkdirAll(filepath.Join(site, "tags"), 0o755); err != nil {
			return err
		}
		body, err := r.RenderTagPage(t, listings)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(site, rel), []byte(body), 0o644); err != nil {
			return err
		}
	}

	if err := r.theme.WriteTo(site); err != nil {
		return err
	}
	if err := copyBundleFiles(r.cfg.Content, site); err != nil {
		return err
	}
	// Only a bundle that actually has a diagram needs the runtime. A missing
	// vendor directory is an error in that case, since the browser would
	// otherwise fetch mermaid from a public CDN and the render is not offline.
	if r.hasMermaid() {
		if err := copyVendor(r.cfg.VendorDir, filepath.Join(site, "vendor")); err != nil {
			return err
		}
	}

	index, err := r.SearchIndexJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "search-index.json"), index, 0o644); err != nil {
		return err
	}

	tags, err := r.TagsJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "tags.json"), tags, 0o644); err != nil {
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
	// whatever the base is. A bundle root holding an activated landing.md
	// introduces the deployment there instead.
	return r.writeLandingOrRedirect(out)
}

// writeRootRedirect points the output root at the base, so serving the output
// directory lands on the wiki. It is skipped when the site is already the output
// root, which is what base "/" produces: the wiki's own index.html sits there,
// and a redirect would replace the home page with a self-redirect. A wiki nested
// in a multi-site tree never writes it either: the tree owns the output root.
func (r *Renderer) writeRootRedirect(out string) error {
	base := r.cfg.base()
	site, err := r.cfg.SiteDir(out)
	if err != nil {
		return err
	}
	if r.nested || filepath.Clean(site) == filepath.Clean(out) {
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
		// A dot-directory is not a section, the same as it is not content.
		if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
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
		// An activated root landing.md introduces the deployment; any other
		// landing.md is an ordinary page.
		if hasLandingType(f) {
			continue
		}
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

// loadSection collects every Markdown file under dir, at any depth. A nested
// directory is not a section of its own: its pages belong to this section and
// carry a slug with the path in it, so guide/setup/install.md under a "guide"
// section is guide/guide/setup/install. The alternative, a one-level glob, drops
// those pages silently while still rewriting links to them, which produces a
// site full of dead links and a render that reports success.
func (r *Renderer) loadSection(dir, id, title string) (Section, error) {
	sec := Section{ID: id, Title: title}
	files, err := markdownFiles(dir)
	if err != nil {
		return sec, err
	}
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return sec, err
		}
		slug := path.Join(id, filepath.ToSlash(strings.TrimSuffix(rel, ".md")))
		p, err := r.parsePage(id, slug, f)
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
	Nav         string   `yaml:"nav"`
	Kicker      string   `yaml:"kicker"`
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Status      string   `yaml:"status"`
	Tags        []string `yaml:"tags"`
	StaleAfter  string   `yaml:"stale_after"`
	Sources     []Source `yaml:"sources"`
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
		StaleAfter:  fm.StaleAfter,
		Sources:     fm.Sources,
		GeneratedAt: fm.Generated.At,
		Title:       fm.Title,
		Nav:         fm.Nav,
		Kicker:      fm.Kicker,
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
			// The table of contents stops at level two. Deeper headings keep
			// their ids as link targets but stay out of the navigation, which
			// is what keeps it short on a deeply nested page.
			if h.Level == 2 {
				p.TOC = append(p.TOC, TOCEntry{ID: id, Text: headingVal})
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
	// Sources rewrite footnotes and append a section, so they run after the
	// heading walk that assigned ids and before anything reads the tree.
	r.applySources(doc, &p, usedIDs)
	p.searchChunks = extractSearchChunks(doc, src)

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

// linkPath resolves a markdown link from a page slug into a wiki URL. A link
// that stays inside the bundle and ends in .md maps to a rendered HTML page. An
// in-bundle link is written either from the bundle root, "/section/page.md",
// which the authoring convention prefers because it survives the page moving, or
// relative to the page. A target that climbs out of the bundle maps to /repo/,
// which only resolves when a repository root was supplied; with none, the href
// stays as written rather than becoming a dead /repo/ URL.
func (r *Renderer) linkPath(fromSlug, href string) string {
	if strings.Contains(href, "://") || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "//") ||
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
	// A leading slash counts from the bundle root, so it does not move with the
	// page. Anything else is relative to the page. Clean clamps a root-relative
	// target at the root, so one can never climb out through a leading slash.
	var clean string
	if strings.HasPrefix(target, "/") {
		clean = strings.TrimPrefix(path.Clean(target), "/")
	} else {
		clean = path.Clean(path.Join(path.Dir(fromSlug), target))
	}

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

// hasMermaid reports whether any page has a diagram in it. The renderer writes
// a mermaid fence through unchanged, as a pre.mermaid block, so the rendered
// body is what to look at.
func (r *Renderer) hasMermaid() bool {
	for _, s := range r.sections {
		for _, p := range s.Pages {
			if strings.Contains(p.Body, `<pre class="mermaid">`) {
				return true
			}
		}
	}
	return false
}

// markdownFiles lists the Markdown files under root, at any depth, as paths
// relative to root. Dotfiles and dot-directories are skipped, for the same
// reason copyBundleFiles skips them.
//
// The root is resolved first: WalkDir does not follow a symlink it is handed as
// its starting point, so a bundle directory that is itself a symlink would
// otherwise yield nothing. Links under the root are followed by the walk as
// usual.
func markdownFiles(root string) ([]string, error) {
	real, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if p != real && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		// The landing name activates only at a bundle root or a multi-site
		// group; anywhere else an activated file is skipped rather than
		// published, so it can only ever mean the introduction.
		if strings.EqualFold(name, landingFile) {
			if hasLandingType(p) {
				return nil
			}
			// Falls through: without the type it is an ordinary page.
		}
		rel, err := filepath.Rel(real, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.Join(root, rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// copyBundleFiles copies the bundle's files into the site, keeping their paths:
// every non-Markdown file, so an image in a page resolves, and every Markdown
// file, so a page can offer its own source for download. The first is load
// bearing because without it an image ships as a broken link and no error; the
// second is what a reader downloads when they want the Markdown.
//
// Dotfiles and dot-directories are skipped: a bundle directory is a real
// working tree as often as not, and nothing in a wiki should be able to publish
// a .git directory or a .env.
//
// The root is resolved first, for the same reason as markdownFiles: WalkDir
// treats a symlinked starting point as a file, so a bundle reached through a
// link would copy nothing, which is how a linked wiki came to serve pages with
// every image broken.
func copyBundleFiles(root, dest string) error {
	real, err := resolveRoot(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(real, p)
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
		// The introduction page is never a wiki page, so its source is not
		// offered beside the pages that are.
		if strings.EqualFold(name, landingFile) {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		return copyFile(p, filepath.Join(dest, rel))
	})
}

// resolveRoot returns the path a walk should start from, following a symlink at
// the root. filepath.WalkDir does not follow the root it is handed, so a bundle
// directory that is itself a symlink would be seen as a single non-directory
// entry. Resolving once makes a linked bundle walk like a real one; links below
// the root are followed by the walk itself.
func resolveRoot(root string) (string, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return real, nil
}

// copyVendor copies the mermaid bundle directory into the output, if present.
// It reports a missing directory rather than returning quietly, because the
// runtime falls back to a public CDN in that case and a silent return made an
// offline render look fine while it was not. A bundle with no diagram does not
// need the runtime, so the caller only asks for it when there is one.
func copyVendor(dir, dest string) error {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("vendor dir %s: no diagram runtime to copy, so the "+
			"browser would fall back to a public CDN and this render is not "+
			"offline: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vendor dir %s: not a directory, so no diagram runtime "+
			"is copied and this render is not offline", dir)
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
