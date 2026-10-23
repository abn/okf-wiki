package wiki

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// agentVersion is the schema version of pages.json. It is separate from the
// graph and search versions because an agent manifest can gain a field without
// changing either of those files.
const agentVersion = 1

// AgentManifest is pages.json: the compact page list an agent can fetch before
// choosing what to read. It carries discovery metadata and links, not page
// text, so it stays small beside the search and graph catalogues.
type AgentManifest struct {
	Version     int         `json:"version"`
	GeneratedAt string      `json:"generatedAt,omitempty"`
	Pages       []AgentPage `json:"pages"`
}

// AgentPage is one content page in the manifest. URL and MarkdownURL are the
// rendered page and its source, relative to the manifest's own base. ID is the
// same page node ID the graph uses, namespaced in a tree aggregate.
type AgentPage struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url"`
	MarkdownURL string         `json:"markdownUrl"`
	Section     string         `json:"section,omitempty"`
	Site        string         `json:"site,omitempty"`
	SiteKey     string         `json:"siteKey,omitempty"`
	Type        string         `json:"type,omitempty"`
	Status      string         `json:"status,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	GeneratedAt string         `json:"generatedAt,omitempty"`
	StaleAfter  string         `json:"staleAfter,omitempty"`
	ContentHash string         `json:"contentHash,omitempty"`
	Headings    []AgentHeading `json:"headings,omitempty"`
}

// AgentHeading is one addressable heading in a page. ID is the rendered anchor,
// so a citation can point at the exact section rather than the page top.
type AgentHeading struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// contentHash is the SHA-256 of a page's Markdown source, including its
// frontmatter. It lets an agent tell whether a cached page is still current
// without comparing the whole text.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// AgentPages returns the content pages of one bundle in render order. Synthetic
// pages such as tag listings and the graph view have no Markdown source, so
// they are not part of the agent manifest.
func (r *Renderer) AgentPages() []AgentPage {
	pages := []AgentPage{}
	for _, s := range r.sections {
		for _, p := range s.Pages {
			if p.Synthetic {
				continue
			}
			pages = append(pages, AgentPage{
				ID:          pageNodeID(p.Slug),
				Title:       pageTitle(p),
				Description: p.Description,
				URL:         r.urlBase() + p.Slug + ".html",
				MarkdownURL: r.urlBase() + p.Slug + ".md",
				Section:     sectionHeading(s),
				Type:        p.Type,
				Status:      p.Status,
				Tags:        p.Tags,
				GeneratedAt: p.GeneratedAt,
				StaleAfter:  p.StaleAfter,
				ContentHash: p.sourceHash,
				Headings:    agentHeadings(p),
			})
		}
	}
	return pages
}

// agentHeadings collects the heading anchors a page exposes, in document order.
// The search chunks already carry every heading the page indexed, including
// levels below the level-two table of contents.
func agentHeadings(p Page) []AgentHeading {
	var out []AgentHeading
	seen := make(map[string]bool)
	for _, chunk := range p.searchChunks {
		if chunk.Heading == "" || chunk.Anchor == "" || seen[chunk.Anchor] {
			continue
		}
		seen[chunk.Anchor] = true
		out = append(out, AgentHeading{ID: chunk.Anchor, Title: chunk.Heading})
	}
	return out
}

// AgentJSON returns the compact pages.json manifest.
func (r *Renderer) AgentJSON() ([]byte, error) {
	return agentJSON(r.AgentPages(), r.generatedAt)
}

func agentJSON(pages []AgentPage, generatedAt string) ([]byte, error) {
	if pages == nil {
		pages = []AgentPage{}
	}
	return json.Marshal(AgentManifest{
		Version:     agentVersion,
		GeneratedAt: generatedAt,
		Pages:       pages,
	})
}

// agentLLMS returns llms.txt for a bundle or tree. It is a discovery document:
// one link per page to the Markdown source, grouped by site and section, with
// the page's description where one exists. It deliberately carries no page
// text, so an agent can choose a page before paying for its content.
func agentLLMS(title, description string, pages []AgentPage) string {
	if strings.TrimSpace(title) == "" {
		title = "Wiki"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(title))
	if description = oneLine(description); description != "" {
		fmt.Fprintf(&b, "> %s\n\n", description)
	}
	b.WriteString("Machine-readable page list: [pages.json](pages.json).\n\n")

	lastSiteKey := "\x00"
	lastSection := "\x00"
	for _, p := range pages {
		siteKey := p.SiteKey
		if siteKey == "" {
			siteKey = p.Site
		}
		if p.Site != "" && siteKey != lastSiteKey {
			ensureBlankLine(&b)
			fmt.Fprintf(&b, "## %s\n\n", oneLine(p.Site))
			lastSiteKey = siteKey
			lastSection = "\x00"
		}
		if p.Section != lastSection {
			level := "##"
			if p.Site != "" {
				level = "###"
			}
			ensureBlankLine(&b)
			fmt.Fprintf(&b, "%s %s\n\n", level, oneLine(p.Section))
			lastSection = p.Section
		}
		line := fmt.Sprintf("- [%s](%s)", escapeLinkText(p.Title), escapeLinkDestination(p.MarkdownURL))
		if desc := oneLine(p.Description); desc != "" {
			line += ": " + desc
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// writeAgentFiles writes pages.json and llms.txt into one site directory.
func writeAgentFiles(dir, title, description, generatedAt string, pages []AgentPage) error {
	manifest, err := agentJSON(pages, generatedAt)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "pages.json"), manifest, 0o644); err != nil {
		return err
	}
	llms := agentLLMS(title, description, pages)
	return os.WriteFile(filepath.Join(dir, "llms.txt"), []byte(llms), 0o644)
}

// writeTreeAgentFiles writes the tree's one agent manifest and discovery file
// at the tree base, after every wiki has rendered.
func (r *Renderer) writeTreeAgentFiles(out string, root *siteNode) error {
	title, description := r.siteMeta(root)
	dir := r.cfg.sitePath(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeAgentFiles(dir, title, description, r.generatedAt, r.aggregatedPages)
}

// agentCollision reports a file in a bundle root that owns a path the agent
// surface needs. Overwriting a reader's llms.txt or pages.json silently would
// be worse than failing the render and naming the file.
func agentCollision(dir string) error {
	for _, name := range []string{"llms.txt", "pages.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("the agent surface needs %s, which is already occupied; rename that file", name)
		}
	}
	return nil
}

func ensureBlankLine(b *strings.Builder) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteByte('\n')
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func escapeLinkDestination(s string) string {
	parts := strings.Split(s, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func escapeLinkText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "[", `\[`)
	s = strings.ReplaceAll(s, "]", `\]`)
	return s
}
