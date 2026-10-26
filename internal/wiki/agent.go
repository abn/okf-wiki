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

// AgentPage is one content page in the manifest. URL and MarkdownURL locate the
// rendered page and its source under the manifest's base, absolute online and
// relative offline. ID is the same page node ID the graph uses, namespaced only
// in a tree's aggregate.
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

// AgentPages returns the content pages of one bundle in render order, with links
// in the form a page of this render uses. Synthetic pages such as tag listings
// and the graph view have no Markdown source, so they are not part of the agent
// manifest.
func (r *Renderer) AgentPages() []AgentPage {
	return r.agentPagesAt(r.urlBase())
}

// agentURLBase is the base the manifest's own links hang off. Online it is the
// wiki's public base, the same as its pages. Offline it is the manifest's own
// directory, because a client resolves a relative link against the file it
// fetched, not against the render root the pages use.
func (r *Renderer) agentURLBase() string {
	if r.cfg.Offline {
		return "./"
	}
	return r.cfg.base()
}

func (r *Renderer) agentPagesAt(base string) []AgentPage {
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
				URL:         base + p.Slug + ".html",
				MarkdownURL: base + p.Slug + ".md",
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

// agentLLMS returns llms.txt for a bundle or one wiki. It is a discovery
// document: one link per page to the Markdown source, grouped by section, with
// the page's description where one exists. It deliberately carries no page
// text, so an agent can choose a page before paying for its content.
func agentLLMS(title, description string, pages []AgentPage) string {
	var b strings.Builder
	writeAgentHeader(&b, title, description)
	b.WriteString("Machine-readable page list: [pages.json](pages.json).\n\n")

	lastSection := "\x00"
	for _, p := range pages {
		if p.Section != lastSection {
			ensureBlankLine(&b)
			fmt.Fprintf(&b, "## %s\n\n", oneLine(p.Section))
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

// writeAgentHeader writes the H1 and summary blockquote every llms.txt starts
// with.
func writeAgentHeader(b *strings.Builder, title, description string) {
	if strings.TrimSpace(title) == "" {
		title = "Wiki"
	}
	fmt.Fprintf(b, "# %s\n\n", oneLine(title))
	if description = oneLine(description); description != "" {
		fmt.Fprintf(b, "> %s\n\n", description)
	}
}

// writeAgentFiles writes pages.json and llms.txt into one wiki directory. Every
// wiki of a tree writes its own, so a client can be pointed at one wiki.
func writeAgentFiles(dir, title, description, generatedAt string, pages []AgentPage) error {
	if err := writeAgentManifest(dir, generatedAt, pages); err != nil {
		return err
	}
	return writeAgentLLMS(dir, agentLLMS(title, description, pages))
}

// writeAgentManifest writes pages.json: the page table without page text.
func writeAgentManifest(dir, generatedAt string, pages []AgentPage) error {
	manifest, err := agentJSON(pages, generatedAt)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "pages.json"), manifest, 0o644)
}

// writeAgentLLMS writes one llms.txt.
func writeAgentLLMS(dir, body string) error {
	return os.WriteFile(filepath.Join(dir, "llms.txt"), []byte(body), 0o644)
}

// agentSiteLink is one rendered wiki of a tree, as the tree-base llms.txt names
// it.
type agentSiteLink struct {
	Rel         string
	Title       string
	Description string
	Pages       int
}

// agentTreeLLMS returns the tree-base llms.txt. It is a router: each line links
// to one wiki's own llms.txt, so an agent reads the page list of the wiki it
// needs rather than every page in the tree. The aggregate pages.json stays
// beside it for a client that wants the whole table at once.
func agentTreeLLMS(title, description string, sites []agentSiteLink) string {
	var b strings.Builder
	writeAgentHeader(&b, title, description)
	b.WriteString("Each line links to one wiki's llms.txt, which lists that wiki's pages. The tree's full page table is [pages.json](pages.json).\n\n")
	if len(sites) == 0 {
		return b.String()
	}
	b.WriteString("## Wikis\n\n")
	for _, s := range sites {
		count := fmt.Sprintf("%d pages", s.Pages)
		if s.Pages == 1 {
			count = "1 page"
		}
		note := count
		if desc := oneLine(s.Description); desc != "" {
			note = desc + " (" + count + ")"
		}
		fmt.Fprintf(&b, "- [%s](%s): %s\n", escapeLinkText(s.Title), escapeLinkDestination(s.Rel+"/llms.txt"), note)
	}
	return b.String()
}

// writeTreeAgentFiles writes the tree's aggregate page table and a router to
// each wiki's own agent files at the tree base, after every wiki has rendered.
func (r *Renderer) writeTreeAgentFiles(out string, root *siteNode) error {
	title, description := r.siteMeta(root)
	dir := r.cfg.sitePath(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeAgentManifest(dir, r.generatedAt, r.aggregatedPages); err != nil {
		return err
	}
	if err := writeAgentLLMS(dir, agentTreeLLMS(title, description, r.treeSites)); err != nil {
		return err
	}
	return writeAgentState(dir, r.previousPages, r.aggregatedPages, r.aggregatedGraph, r.generatedAt, r.crossBroken...)
}

// agentCollision reports a file in a bundle root that owns a path the agent
// surface needs. Overwriting a reader's llms.txt or pages.json silently would
// be worse than failing the render and naming the file.
func agentCollision(dir string) error {
	for _, name := range []string{"llms.txt", "pages.json", "changes.json", "health.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("the agent surface needs %s in %s, which is already occupied; rename that file", name, dir)
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
