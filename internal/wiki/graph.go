package wiki

import (
	"encoding/json"
	"path"
	"strings"
)

// graphVersion is the schema version of graph.json. A reader checks it before
// relying on a field, so a later shape can ship beside this one instead of
// being guessed at.
const graphVersion = 1

// pageLink is one in-bundle Markdown link found while parsing a page. To is the
// target's page slug, Anchor the heading the link sits under, and Text what the
// reader sees. It is resolved into a graph edge after every page is known, so a
// target the bundle does not contain can become a stub instead of a dead edge.
type pageLink struct {
	To     string
	Anchor string
	Text   string
}

// pageTitle is the page's display title, falling back to its slug when neither
// frontmatter nor the body supplied one.
func pageTitle(p Page) string {
	if p.Title != "" {
		return p.Title
	}
	return p.Slug
}

// sectionHeading is a section's display title, with the fallback the
// navigation uses for the section that holds root-level pages.
func sectionHeading(s Section) string {
	if s.Title != "" {
		return s.Title
	}
	return "Overview"
}

// GraphNode is one page or tag. ID is "page:" plus the page slug, or "tag:"
// plus the tag slug, so the two kinds share one ID space without a file named
// "tag:foo.md" colliding with the hub for a tag called foo. A stub is a page
// that an in-bundle link names and the bundle does not contain: it has no URL
// and exists so the broken link is in the catalogue.
type GraphNode struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	URL     string   `json:"url,omitempty"`
	Kind    string   `json:"kind"` // "page" or "tag"
	Section string   `json:"section,omitempty"`
	Type    string   `json:"type,omitempty"`
	Status  string   `json:"status,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Stub    bool     `json:"stub,omitempty"`
	// Site and SiteKey are set only in a tree's aggregate, where they name the
	// wiki a node came from and its path under the tree root.
	Site    string `json:"site,omitempty"`
	SiteKey string `json:"siteKey,omitempty"`
}

// GraphEdge is one relation. Kind is "link" for a Markdown link, carrying the
// heading anchor and the link text that place it in the page, or "tag" for a
// frontmatter tag.
type GraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Anchor string `json:"anchor,omitempty"`
	Text   string `json:"text,omitempty"`
}

// GraphIndex is graph.json: the structural graph of a bundle, or of a tree with
// each wiki's IDs namespaced.
type GraphIndex struct {
	Version int         `json:"version"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

// pageNodeID is the catalogue ID of a page. Pages and tags share one ID space,
// so both carry a kind prefix: a page file may legally be named "tag:foo.md",
// and a raw slug would then collide with the hub for a tag called foo.
func pageNodeID(slug string) string { return "page:" + slug }

// BuildGraph collects one node per page and per tag, a link edge for every
// in-bundle Markdown link, and a tag edge for every frontmatter tag. An
// in-bundle link whose target the bundle does not contain becomes a stub node,
// so a broken link is visible rather than silently missing. Order follows the
// render's page order, which keeps the file byte-stable for an unchanged
// bundle.
func (r *Renderer) BuildGraph() GraphIndex {
	g := GraphIndex{Version: graphVersion, Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	byID := make(map[string]int)

	for _, s := range r.sections {
		for _, p := range s.Pages {
			id := pageNodeID(p.Slug)
			byID[id] = len(g.Nodes)
			g.Nodes = append(g.Nodes, GraphNode{
				ID:      id,
				Title:   pageTitle(p),
				URL:     r.cfg.base() + p.Slug + ".html",
				Kind:    "page",
				Section: sectionHeading(s),
				Type:    p.Type,
				Status:  p.Status,
				Tags:    p.Tags,
			})
		}
	}

	for _, s := range r.sections {
		for _, p := range s.Pages {
			from := pageNodeID(p.Slug)
			seen := make(map[string]bool)
			for _, l := range p.links {
				key := "link\x00" + l.To + "\x00" + l.Anchor
				if seen[key] {
					continue
				}
				seen[key] = true
				to, ok := byID[pageNodeID(l.To)]
				if !ok {
					to = addStubNode(&g, byID, l.To)
				}
				g.Edges = append(g.Edges, GraphEdge{
					From:   from,
					To:     g.Nodes[to].ID,
					Kind:   "link",
					Anchor: l.Anchor,
					Text:   l.Text,
				})
			}
			for _, tag := range p.Tags {
				name := strings.TrimSpace(tag)
				slug := slugify(name)
				if slug == "" {
					continue
				}
				id := "tag:" + slug
				if _, ok := byID[id]; !ok {
					byID[id] = len(g.Nodes)
					g.Nodes = append(g.Nodes, GraphNode{
						ID:    id,
						Title: "#" + name,
						URL:   r.cfg.base() + "tags/" + slug + ".html",
						Kind:  "tag",
					})
				}
				key := "tag\x00" + id
				if seen[key] {
					continue
				}
				seen[key] = true
				g.Edges = append(g.Edges, GraphEdge{From: from, To: id, Kind: "tag"})
			}
		}
	}
	return g
}

// addStubNode records a page an in-bundle link names but the bundle does not
// contain. The title is the last path segment, so a reader sees a page name
// rather than the whole slug.
func addStubNode(g *GraphIndex, byID map[string]int, slug string) int {
	title := path.Base(slug)
	if title == "index" || title == "." {
		if dir := path.Base(path.Dir(slug)); dir != "." && dir != "/" {
			title = dir
		} else {
			title = slug
		}
	}
	id := pageNodeID(slug)
	byID[id] = len(g.Nodes)
	g.Nodes = append(g.Nodes, GraphNode{ID: id, Title: title, Kind: "page", Stub: true})
	return len(g.Nodes) - 1
}

// GraphJSON returns the JSON representation of the structural graph.
func (r *Renderer) GraphJSON() ([]byte, error) {
	return json.Marshal(r.BuildGraph())
}

// markdownTarget reports whether href names a Markdown file, which is what
// makes a link a page edge. A fragment or query is ignored; external,
// protocol-relative and fragment-only links are not Markdown targets.
func markdownTarget(href string) bool {
	if strings.Contains(href, "://") || strings.HasPrefix(href, "//") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "#") {
		return false
	}
	target, _, _ := strings.Cut(href, "#")
	target, _, _ = strings.Cut(target, "?")
	return strings.HasSuffix(target, ".md")
}

// graphTarget returns the page slug a rewritten link points at. Only a URL under
// this render's base that ends in .html is a page: an external link, a /repo/
// target and an in-bundle asset each fail the prefix or the suffix.
func (r *Renderer) graphTarget(dest string) (string, bool) {
	base := r.cfg.base()
	if !strings.HasPrefix(dest, base) {
		return "", false
	}
	slug := strings.TrimPrefix(dest, base)
	if i := strings.IndexByte(slug, '#'); i >= 0 {
		slug = slug[:i]
	}
	if !strings.HasSuffix(slug, ".html") {
		return "", false
	}
	slug = strings.TrimSuffix(slug, ".html")
	if slug == "" || strings.HasPrefix(slug, "..") {
		return "", false
	}
	return slug, true
}

// mergeGraph appends src to dst, labelling every node with its wiki and
// prefixing every ID with key, the wiki's path under the tree root. IDs are
// unique only within a wiki, so the prefix keeps two wikis' page and tag nodes
// apart. Node URLs already carry the wiki's base and are left alone. An empty
// key is not a wiki of a tree and is refused, so a caller cannot merge two
// wikis into one ID space.
func mergeGraph(dst, src *GraphIndex, site, key string) {
	if len(src.Nodes) == 0 || key == "" {
		return
	}
	if dst.Version == 0 {
		dst.Version = graphVersion
	}
	prefix := key + ":"
	for i := range src.Nodes {
		src.Nodes[i].Site = site
		src.Nodes[i].SiteKey = key
		src.Nodes[i].ID = prefix + src.Nodes[i].ID
	}
	for i := range src.Edges {
		src.Edges[i].From = prefix + src.Edges[i].From
		src.Edges[i].To = prefix + src.Edges[i].To
	}
	dst.Nodes = append(dst.Nodes, src.Nodes...)
	dst.Edges = append(dst.Edges, src.Edges...)
}
