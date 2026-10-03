// Command okf-wiki renders an Open Knowledge Format (OKF v0.2) bundle into a
// branded, searchable static wiki and serves it locally. It is designed to run
// either directly or as a container: point it at a mounted content directory and
// it renders and serves the whole bundle.
//
//	okf-wiki serve --content ./docs
//	okf-wiki render --content ./docs --out ./public
//
// Configuration resolves flags > environment (OKF_WIKI_*) > defaults. If
// --content is omitted, the content directory is auto-detected from the
// container's bind mounts.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abn/okf-wiki/internal/wiki"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "render":
		runRender(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "detect":
		runDetect(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("okf-wiki %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "okf-wiki: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `okf-wiki `+version+` - render and serve an OKF wiki

usage:
  okf-wiki serve  [CONTENT [OUT]] [flags]   render, then serve until interrupted
  okf-wiki render [CONTENT [OUT]] [flags]   render only
  okf-wiki detect [flags]   print the auto-detected content directory
  okf-wiki version

  render and serve accept the content directory and the output directory
  positionally, in that order, and they may be mixed with flags in any order.
  A flag or an environment variable still wins.

flags (render):
  --content DIR      OKF bundle to render (auto-detected if omitted)
  --out DIR          output directory (default: .scratch/wiki)
  --base PATH        URL prefix the wiki is served under (default "/wiki/")
  --repo DIR         repository root exposed read-only at /repo/ (optional)
  --vendor DIR       directory holding the mermaid bundle (required if the
                     bundle has a diagram, or the render is not offline)
  --theme DIR        theme directory layered over the embedded default (optional)
  --brand STR        brand wordmark (default "okf-wiki")
  --brand-sub STR    brand subtitle (default "docs")
  --version-tag STR  freeform version label beside the wordmark (optional)
  --title STR        document title suffix (default "Wiki")
  --sections LIST    section order as id:Title pairs, comma-separated
  --nav-links LIST   header links as Label=URL pairs, comma-separated

flags (serve adds):
  --addr HOST:PORT listen address (default 127.0.0.1:8080, loopback only)
  --open           open a browser (default false; off in containers)
  --watch          re-render when the bundle or theme changes

serve drains in-flight requests on SIGINT or SIGTERM before exiting, which is
what a container stop sends. A request part way through a body is finished
rather than cut.

environment: OKF_WIKI_CONTENT, OKF_WIKI_OUT, OKF_WIKI_BASE, OKF_WIKI_REPO,
  OKF_WIKI_VENDOR, OKF_WIKI_THEME, OKF_WIKI_BRAND, OKF_WIKI_BRAND_SUB,
  OKF_WIKI_VERSION_TAG, OKF_WIKI_TITLE, OKF_WIKI_SECTIONS, OKF_WIKI_NAV_LINKS,
  OKF_WIKI_ADDR, OKF_WIKI_OPEN, OKF_WIKI_WATCH
`)
}

func runRender(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	cfg := bindCommon(fs)
	fs.Parse(reorderArgs(args, fs))
	applyPositional(fs, cfg)
	if err := doRender(cfg); err != nil {
		fatal(err)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfg := bindCommon(fs)
	addr := fs.String("addr", env("OKF_WIKI_ADDR", wiki.DefaultAddr), "listen address")
	open := fs.Bool("open", envBool("OKF_WIKI_OPEN", false), "open a browser")
	watch := fs.Bool("watch", envBool("OKF_WIKI_WATCH", false), "re-render when the bundle or theme changes")
	fs.Parse(reorderArgs(args, fs))
	applyPositional(fs, cfg)
	if err := doRender(cfg); err != nil {
		fatal(err)
	}
	if *watch {
		// The server reads from disk on every request and asks for
		// revalidation, so a re-render is live on the next load with nothing
		// restarted. The goroutine lives as long as the process; serve blocks
		// until a signal, and the process ends after it.
		go wiki.Watch(context.Background(), wiki.WatchOptions{
			Content:  cfg.Content,
			ThemeDir: cfg.ThemeDir,
			Render:   func() error { return reRender(*cfg) },
		})
		fmt.Println("okf-wiki: watching the bundle and theme for changes")
	}
	absRepo := ""
	if cfg.Repo != "" {
		absRepo, _ = absPath(cfg.Repo)
	}
	if err := wiki.Serve(wiki.ServeOptions{
		OutDir:   cfg.Out,
		Base:     cfg.Base,
		RepoRoot: absRepo,
		Addr:     *addr,
		Open:     *open,
	}); err != nil {
		fatal(err)
	}
}

// reRender renders the configured bundle again into its output directory. It is
// the reload path, so it stays quiet on success: the next request shows the
// result, and Watch reports a failure.
func reRender(cfg wiki.Config) error {
	r, err := wiki.New(cfg)
	if err != nil {
		return err
	}
	return r.RenderAll(cfg.Out)
}

func runDetect(args []string) {
	fs := flag.NewFlagSet("detect", flag.ExitOnError)
	fs.Parse(args)
	dir, err := wiki.DetectContent(nil)
	if err != nil {
		fatal(err)
	}
	fmt.Println(dir)
}

func bindCommon(fs *flag.FlagSet) *wiki.Config {
	c := &wiki.Config{}
	fs.StringVar(&c.Content, "content", env("OKF_WIKI_CONTENT", ""), "OKF bundle to render")
	fs.StringVar(&c.Out, "out", env("OKF_WIKI_OUT", ".scratch/wiki"), "output directory")
	fs.StringVar(&c.Base, "base", env("OKF_WIKI_BASE", wiki.DefaultBase), "URL prefix the wiki is served under")
	fs.StringVar(&c.Repo, "repo", env("OKF_WIKI_REPO", ""), "repository root for /repo/")
	fs.StringVar(&c.VendorDir, "vendor", env("OKF_WIKI_VENDOR", "mermaid"), "mermaid bundle directory")
	fs.StringVar(&c.ThemeDir, "theme", env("OKF_WIKI_THEME", ""), "theme directory layered over the embedded default")
	fs.StringVar(&c.Brand.Name, "brand", env("OKF_WIKI_BRAND", "okf-wiki"), "brand wordmark")
	fs.StringVar(&c.Brand.Sub, "brand-sub", env("OKF_WIKI_BRAND_SUB", "docs"), "brand subtitle")
	fs.StringVar(&c.Brand.VersionTag, "version-tag", env("OKF_WIKI_VERSION_TAG", ""), "freeform version label beside the wordmark")
	fs.StringVar(&c.Brand.Title, "title", env("OKF_WIKI_TITLE", "Wiki"), "document title suffix")
	fs.StringVar(&c.Brand.Sections, "sections", env("OKF_WIKI_SECTIONS", ""), "section order as id:Title pairs")
	fs.StringVar(&c.Brand.NavLinks, "nav-links", env("OKF_WIKI_NAV_LINKS", ""), "header links as Label=URL pairs")
	return c
}

// doRender renders the configured bundle into its output directory. It resolves
// the content directory when none was given and records it back on cfg, so a
// caller that then watches the bundle knows what to watch.
func doRender(cfg *wiki.Config) error {
	if cfg.Content == "" {
		detected, err := wiki.DetectContent(nil)
		if err != nil {
			return fmt.Errorf("no --content given and auto-detection failed: %w", err)
		}
		fmt.Printf("okf-wiki: auto-detected content at %s\n", detected)
		cfg.Content = detected
	}
	r, err := wiki.New(*cfg)
	if err != nil {
		return err
	}
	if err := r.RenderAll(cfg.Out); err != nil {
		return err
	}
	total := 0
	for _, s := range r.Sections() {
		total += len(s.Pages)
	}
	theme := r.Theme()
	site, err := cfg.SiteDir(cfg.Out)
	if err != nil {
		return err
	}
	fmt.Printf("okf-wiki: rendered %d pages from %s -> %s\n", total, cfg.Content, site)
	fmt.Printf("okf-wiki: theme %s %s (%s), base %s\n", theme.Name, theme.Version, theme.Source, cfg.Base)
	return nil
}

// applyPositional fills Content and Out from the positional arguments, which
// are accepted so the paths can be given without their flag names.
//
// Whether a value was stated explicitly is decided by fs.Visit, not by testing
// the resolved value. The default for --out is a real path, so the old
// `cfg.Out == ""` test could never be true and the second positional was
// silently discarded while the command still reported success. An explicit
// --out or OKF_WIKI_OUT still wins, which is the documented precedence.
func applyPositional(fs *flag.FlagSet, cfg *wiki.Config) {
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if _, ok := os.LookupEnv("OKF_WIKI_CONTENT"); ok {
		given["content"] = true
	}
	if _, ok := os.LookupEnv("OKF_WIKI_OUT"); ok {
		given["out"] = true
	}
	if fs.NArg() > 0 && !given["content"] {
		cfg.Content = fs.Arg(0)
	}
	if fs.NArg() > 1 && !given["out"] {
		cfg.Out = fs.Arg(1)
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "okf-wiki: %v\n", err)
	os.Exit(1)
}

// reorderArgs lets positional paths coexist with flags regardless of order.
func reorderArgs(args []string, fs *flag.FlagSet) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 0 && a[0] == '-' {
			flags = append(flags, a)
			if !strings.ContainsRune(a, '=') && i+1 < len(args) && !isBoolFlag(fs, a) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

// isBoolFlag reports whether a flag takes no value, so the argument after it is
// a positional rather than its value. It asks the flag set rather than keeping a
// list: the list drifted, and --watch and --multi-site were boolean flags it did
// not know, so each ate the argument that followed it.
func isBoolFlag(fs *flag.FlagSet, a string) bool {
	name := strings.TrimLeft(a, "-")
	if f := fs.Lookup(name); f != nil {
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
			return b.IsBoolFlag()
		}
		return false
	}
	// -h and --help are answered by the flag package itself rather than
	// declared, and take no value.
	return name == "h" || name == "help"
}

func absPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	return filepath.Abs(p)
}
