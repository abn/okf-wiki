package wiki

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultBase is the URL prefix the rendered wiki is served under.
const DefaultBase = "/wiki/"

// Brand holds the presentation identity of the rendered wiki.
type Brand struct {
	// Name is the wordmark, e.g. "abn." A trailing "." is split out and handed
	// to the template as BrandTail, which a theme may paint in its accent. The
	// split is mechanical; how the tail looks is the theme's business.
	Name string
	// Sub is the small label beside the wordmark, e.g. "wiki".
	Sub string
	// Title is the document <title> suffix, e.g. "Wiki".
	Title string
	// Sections is a comma-separated order list of "id:Title" pairs. Sections
	// not listed are appended, title-cased from their directory name.
	Sections string
}

// Config is the resolved configuration for a render/serve run.
type Config struct {
	Content   string // OKF bundle directory
	Out       string // output directory
	Base      string // URL prefix the wiki is served under (default "/wiki/")
	Repo      string // repository root exposed at /repo/ (optional)
	VendorDir string // directory holding the mermaid bundle (optional)
	ThemeDir  string // theme override directory (optional)
	Brand     Brand
}

// base returns the URL prefix, normalised to a leading and a trailing slash so
// callers can concatenate a relative path onto it.
func (c Config) base() string {
	if strings.TrimSpace(c.Base) == "" {
		return DefaultBase
	}
	b := c.Base
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	if !strings.HasSuffix(b, "/") {
		b += "/"
	}
	return b
}

// SiteDir returns the directory a render writes into, given the output root.
// The base is both the URL prefix and the on-disk path, so a rendered directory
// can be dropped behind any static file server unchanged: the pages are already
// where the links say they are.
func (c Config) SiteDir(out string) (string, error) {
	b := c.base()
	trimmed := strings.Trim(b, "/")
	if trimmed == "" {
		return out, nil
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("base %q must not contain %q segments", c.Base, seg)
		}
	}
	return filepath.Join(out, filepath.FromSlash(trimmed)), nil
}

// sectionSpec is a parsed entry from Brand.Sections.
type sectionSpec struct {
	ID    string
	Title string
}

// parseSections parses "id:Title,id2:Title2" into an ordered list.
func parseSections(s string) []sectionSpec {
	var out []sectionSpec
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, title, found := strings.Cut(part, ":")
		id = strings.TrimSpace(id)
		title = strings.TrimSpace(title)
		if id == "" {
			continue
		}
		if !found || title == "" {
			title = titleCase(id)
		}
		out = append(out, sectionSpec{ID: id, Title: title})
	}
	return out
}

// titleCase turns a directory name (hyphens/underscores) into a display title.
func titleCase(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// orderedSectionIDs merges the configured order with any directories present in
// the bundle, so an unlisted section always renders. The top-level section (id
// "") is always first, since the bundle root holds index.md and log.md.
func orderedSectionIDs(configured []sectionSpec, present []string) []sectionSpec {
	topTitle := "Overview"
	for _, s := range configured {
		if s.ID == "" {
			topTitle = s.Title
			break
		}
	}
	var out []sectionSpec
	seen := map[string]bool{"": true}
	out = append(out, sectionSpec{ID: "", Title: topTitle})

	for _, s := range configured {
		if s.ID == "" || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	var extra []string
	for _, id := range present {
		if id == "" || seen[id] {
			continue
		}
		extra = append(extra, id)
	}
	sort.Strings(extra)
	for _, id := range extra {
		out = append(out, sectionSpec{ID: id, Title: titleCase(id)})
	}
	return out
}

func (b Brand) validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("brand name must not be empty")
	}
	return nil
}

// resolvedManifest is the theme.json written into the output, so a published
// site records the theme it was rendered with.
type resolvedManifest struct {
	Name        string                  `json:"name"`
	Version     string                  `json:"version"`
	Description string                  `json:"description,omitempty"`
	Source      string                  `json:"source,omitempty"`
	Assets      map[string]themeAsset   `json:"assets"`
	Slots       map[string]resolvedSlot `json:"slots,omitempty"`
	Base        string                  `json:"base"`
	RenderedBy  string                  `json:"renderedBy"`
}

type resolvedSlot struct {
	Bytes int `json:"bytes"`
}

// describe renders the resolved manifest for the output directory.
func (t *Theme) describe(base string) ([]byte, error) {
	assets := map[string]themeAsset{}
	for _, f := range t.files {
		assets[f.Path] = themeAsset{Mode: modeReplace}
	}
	slots := map[string]resolvedSlot{}
	for name, text := range t.slotText {
		slots[name] = resolvedSlot{Bytes: len(text)}
	}
	m := resolvedManifest{
		Name:        t.Name,
		Version:     t.Version,
		Description: t.Description,
		Assets:      assets,
		Base:        base,
		RenderedBy:  "okf-wiki",
	}
	if t.Source != "embedded" {
		m.Source = "override"
	}
	if len(slots) > 0 {
		m.Slots = slots
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
