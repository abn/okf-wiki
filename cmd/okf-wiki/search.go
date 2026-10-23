package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abn/okf-wiki/internal/wiki"
)

// runSearch queries a rendered site's search catalogue. It reads the files the
// browser already fetches, so a search costs no render and returns the same
// ranked chunks with exact snippets and page citations.
func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	out := fs.String("out", env("OKF_WIKI_OUT", ".scratch/wiki"), "render output directory")
	base := fs.String("base", env("OKF_WIKI_BASE", wiki.DefaultBase), "URL prefix the wiki was rendered under")
	jsonOut := fs.Bool("json", false, "write JSON")
	limit := fs.Int("limit", 5, "maximum results, 1 through 20")
	site := fs.String("site", "", "filter by wiki title or site key")
	tag := fs.String("tag", "", "filter by page tag")
	section := fs.String("section", "", "filter by section title")
	fs.Parse(reorderArgs(args, fs))

	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "okf-wiki search: a query is required")
		os.Exit(2)
	}
	if *limit < 1 || *limit > 20 {
		fmt.Fprintln(os.Stderr, "okf-wiki search: --limit must be between 1 and 20")
		os.Exit(2)
	}
	siteDir, err := (wiki.Config{Out: *out, Base: *base}).SiteDir(*out)
	if err != nil {
		fatal(err)
	}
	corpus, err := wiki.LoadSearchCorpus(siteDir)
	if err != nil {
		fatal(fmt.Errorf("load rendered search data: %w", err))
	}
	resp, err := corpus.Search(query, wiki.SearchOptions{
		Limit: *limit, Site: *site, Tag: *tag, Section: *section,
	})
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		body, err := json.Marshal(resp)
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(body))
		return
	}
	if resp.Total == 0 {
		fmt.Printf("No results for %q.\n", resp.Query)
		return
	}
	fmt.Printf("%d result%s for %q.\n", resp.Total, plural(resp.Total), resp.Query)
	for i, r := range resp.Results {
		heading := r.PageTitle
		if r.Title != "" && r.Title != r.PageTitle {
			heading += " / " + r.Title
		}
		fmt.Printf("\n%d. %s\n   %s\n", i+1, heading, r.URL)
		if r.Snippet != "" {
			fmt.Printf("   %s\n", r.Snippet)
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
