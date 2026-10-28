package wiki

import (
	"strings"
	"unicode/utf8"
)

// TitleWarningMax is the length past which a title stops working as a name in
// the sidebar, the graph, a browser tab, and a search result. The graph
// shortens a label at labelMax so its own layout survives; this is the point
// where the title itself needs the author's hand.
const TitleWarningMax = 80

// TitleWarning is one page whose title cannot work as a name. Page is the slug,
// prefixed with the wiki path for a page of a tree.
type TitleWarning struct {
	Page      string
	Length    int
	LineBreak bool
}

// TitleWarnings names every page whose title cannot work as a name. A single
// bundle reads its own pages; a tree collects one list while it renders, and
// returns it from the tree renderer.
func (r *Renderer) TitleWarnings() []TitleWarning { return r.titleWarnings }

// checkTitles records the warnings for the pages this renderer just loaded. A
// line break is worth its own message, because a length count does not explain
// why the title broke the layout.
func (r *Renderer) checkTitles() {
	for _, s := range r.sections {
		for _, p := range s.Pages {
			title := pageTitle(p)
			warning := TitleWarning{
				Page:      p.Slug,
				Length:    utf8.RuneCountInString(title),
				LineBreak: strings.ContainsAny(title, "\r\n"),
			}
			if r.siteRel != "" {
				warning.Page = r.siteRel + "/" + warning.Page
			}
			if warning.Length > TitleWarningMax || warning.LineBreak {
				r.titleWarnings = append(r.titleWarnings, warning)
			}
		}
	}
}
