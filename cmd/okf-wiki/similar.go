package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/abn/okf-wiki/internal/wiki"
)

// runSimilar plans a capture against a bundle: it answers whether a draft
// belongs to a page that already exists or starts a new subject. It reads a
// a single bundle, not a render and not a tree, so it can run before the page is
// written.
func runSimilar(args []string) {
	fs := flag.NewFlagSet("similar", flag.ExitOnError)
	content := fs.String("content", env("OKF_WIKI_CONTENT", ""), "OKF bundle to plan against")
	base := fs.String("base", env("OKF_WIKI_BASE", wiki.DefaultBase), "URL prefix the bundle is rendered under")
	textFile := fs.String("text", "", `draft file, or - for stdin`)
	top := fs.Int("top", 5, "number of subjects, 1 through 20")
	minScore := fs.Float64("min-score", 0, "update threshold, 0 uses the default")
	jsonOut := fs.Bool("json", false, "write JSON")
	fs.Parse(reorderArgs(args, fs))

	if *top < 1 || *top > 20 {
		fmt.Fprintln(os.Stderr, "okf-wiki similar: --top must be between 1 and 20")
		os.Exit(2)
	}
	if math.IsNaN(*minScore) || *minScore < 0 || *minScore > 1 {
		fmt.Fprintln(os.Stderr, "okf-wiki similar: --min-score must be between 0 and 1")
		os.Exit(2)
	}
	draft, err := readDraft(*textFile, fs.Args())
	if err != nil {
		fatal(err)
	}
	if strings.TrimSpace(draft) == "" {
		fmt.Fprintln(os.Stderr, "okf-wiki similar: a draft is required, with --text or as words")
		os.Exit(2)
	}
	cfg := wiki.Config{
		Content: *content,
		Base:    *base,
		// Planning never renders a page, so the brand is a placeholder rather
		// than a presentation choice.
		Brand: wiki.Brand{Name: "okf-wiki"},
	}
	if cfg.Content == "" {
		detected, err := wiki.DetectContent(nil)
		if err != nil {
			fatal(fmt.Errorf("no --content given and auto-detection failed: %w", err))
		}
		cfg.Content = detected
	}
	r, err := wiki.New(cfg)
	if err != nil {
		fatal(err)
	}
	resp := wiki.SimilarPagesWithText(r.AgentPages(), r.PageBodies(), draft, wiki.SimilarOptions{
		Limit: *top, MinScore: *minScore,
	})
	if *jsonOut {
		body, err := json.Marshal(resp)
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(body))
		return
	}
	printSimilar(resp)
}

// readDraft stops one byte past the planner's limit, so a huge file or pipe
// costs the same as a page and the planner can still report the cut.
func readDraft(path string, words []string) (string, error) {
	limit := int64(wiki.SimilarMaxBytes + 1)
	switch {
	case path == "-":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, limit))
		return string(b), err
	case path != "":
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, limit))
		return string(b), err
	default:
		return strings.Join(words, " "), nil
	}
}

func printSimilar(resp wiki.SimilarResponse) {
	if resp.Verdict == "update" {
		fmt.Printf("Update: the best subject matches at %.3f, above the %.3f threshold.\n", resp.Candidates[0].Score, resp.Threshold)
	} else if len(resp.Candidates) > 0 {
		fmt.Printf("New subject: the best match is %.3f, below the %.3f threshold.\n", resp.Candidates[0].Score, resp.Threshold)
	} else {
		fmt.Printf("New subject: nothing in the bundle matches (threshold %.3f).\n", resp.Threshold)
	}
	for i, c := range resp.Candidates {
		name := c.Title
		if name == "" {
			name = c.Subject
		}
		fmt.Printf("\n%d. %s\n   %s  %.3f\n", i+1, name, c.Subject, c.Score)
		if len(c.Pages) > 0 {
			fmt.Printf("   read: %s\n", c.Pages[0].MarkdownURL)
		}
		roles := make([]string, 0, len(c.Pages))
		for _, p := range c.Pages {
			if p.Role != "" {
				roles = append(roles, p.Role)
			}
		}
		if len(roles) > 0 {
			fmt.Printf("   pages: %s\n", strings.Join(roles, ", "))
		}
		if len(c.Matched) > 0 {
			fmt.Printf("   matched stems: %s\n", strings.Join(c.Matched, ", "))
		}
	}
}
