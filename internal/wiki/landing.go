package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// landingFile is the optional page a bundle root or a multi-site group holds
// to introduce the deployment. A single bundle renders it at the output root
// instead of the redirect to the wiki; a group shows its body above the
// generated catalog. The type activates it: only a landing.md carrying
// `type: Landing` introduces the deployment, and any other landing.md is an
// ordinary page, so no bundle needs renaming and no flag opts out.
const landingFile = "landing.md"

// landingType is the frontmatter type that activates the introduction.
const landingType = "Landing"

// landingMarker closes a rendered landing page at the output root. Serve tells
// a landing apart from the redirect written there otherwise by this marker
// rather than by a sentinel file, so adding or removing landing.md under
// --watch takes effect on the next load with no restart.
const landingMarker = "<!-- okf-wiki:landing -->"

// hasLanding checks dir for an activated introduction file.
func hasLanding(dir string) bool {
	return hasLandingType(filepath.Join(dir, landingFile))
}

// hasLandingType reports whether path is a landing.md carrying the activating
// type. A missing or unparsable file is not one: the page stays an ordinary
// page rather than failing the render.
func hasLandingType(path string) bool {
	if !strings.EqualFold(filepath.Base(path), landingFile) {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fm, _ := splitFrontmatter(b)
	return fm.Type == landingType
}

// loadLandingPage parses dir's landing page. Links resolve from the bundle
// root the way header links do, so authors write root-relative targets. The
// page is synthetic: it has no wiki URL of its own, so there is nothing to
// download.
func (r *Renderer) loadLandingPage(dir string) (Page, error) {
	p, err := r.parsePage("", "", filepath.Join(dir, landingFile))
	if err != nil {
		return Page{}, err
	}
	p.Synthetic = true
	if p.Title == "" {
		p.Title = r.cfg.Brand.Title
	}
	return p, nil
}

// renderLandingPage renders dir's landing page through the theme shell with no
// sections, so the sidebar is empty and the layout collapses to its solo
// column. The brand points at the deployment home the page itself sits at,
// and the marker lets serve recognize the page later.
func (r *Renderer) renderLandingPage(dir string) (string, error) {
	p, err := r.loadLandingPage(dir)
	if err != nil {
		return "", err
	}
	shell := &Renderer{
		md:           r.md,
		cfg:          r.cfg,
		theme:        r.theme,
		generatedAt:  r.generatedAt,
		assetVersion: r.assetVersion,
		searchBase:   r.searchBase,
		landing:      true,
	}
	html, err := shell.RenderPage(p)
	if err != nil {
		return "", err
	}
	return html + "\n" + landingMarker + "\n", nil
}

// hasLandingRoot reports whether the output root holds a rendered landing page
// rather than the redirect to the wiki base. Only the output root's index is
// ever read, and only for a request to "/" itself.
func hasLandingRoot(outDir string) bool {
	b, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), landingMarker)
}

// writeLandingOrRedirect publishes the output root's index: the landing page
// when the bundle root holds one, else the redirect to the wiki base. A base
// of "/" leaves no room for both: the wiki home already sits at the output
// root, so claiming it for a landing fails the render and names the clash
// instead of silently replacing the home page.
func (r *Renderer) writeLandingOrRedirect(out string) error {
	if r.nested || !hasLanding(r.cfg.Content) {
		return r.writeRootRedirect(out)
	}
	if strings.Trim(r.cfg.base(), "/") == "" {
		return fmt.Errorf("landing.md with base %q: the wiki home already sits at the output root, so serve the wiki under a base such as /wiki/ to make room for a landing page", r.cfg.Base)
	}
	body, err := r.renderLandingPage(r.cfg.Content)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "index.html"), []byte(body), 0o644)
}
