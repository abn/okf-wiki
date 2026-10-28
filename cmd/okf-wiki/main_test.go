package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abn/okf-wiki/internal/wiki"
)

// The second positional argument used to be unreachable. --out has a real
// default path, so the `cfg.Out == ""` test that guarded the assignment was
// never true, and `okf-wiki render BUNDLE OUT` rendered into the default
// instead while reporting success.
func TestApplyPositional(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		content string
		out     string
	}{
		{
			name:    "flags win over positional",
			args:    []string{"--content", "flag-content", "--out", "flag-out", "pos-content", "pos-out"},
			content: "flag-content",
			out:     "flag-out",
		},
		{
			name:    "positional fills both",
			args:    []string{"pos-content", "pos-out"},
			content: "pos-content",
			out:     "pos-out",
		},
		{
			name:    "positional content only",
			args:    []string{"pos-content"},
			content: "pos-content",
			out:     "default-out",
		},
		{
			name:    "explicit out keeps its value while content comes from positional",
			args:    []string{"--out", "flag-out", "pos-content"},
			content: "pos-content",
			out:     "flag-out",
		},
		{
			name:    "environment beats positional",
			args:    []string{"pos-content", "pos-out"},
			env:     map[string]string{"OKF_WIKI_CONTENT": "env-content", "OKF_WIKI_OUT": "env-out"},
			content: "env-content",
			out:     "env-out",
		},
		{
			name:    "environment out with positional content",
			args:    []string{"pos-content", "pos-out"},
			env:     map[string]string{"OKF_WIKI_OUT": "env-out"},
			content: "pos-content",
			out:     "env-out",
		},
		{
			name:    "no positionals leaves the flag values alone",
			args:    nil,
			content: "default-content",
			out:     "default-out",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(nil)
			cfg := &wiki.Config{
				Content: "default-content",
				Out:     "default-out",
			}
			fs.StringVar(&cfg.Content, "content", env("OKF_WIKI_CONTENT", "default-content"), "")
			fs.StringVar(&cfg.Out, "out", env("OKF_WIKI_OUT", "default-out"), "")

			if err := fs.Parse(reorderArgs(tc.args, fs)); err != nil {
				t.Fatal(err)
			}
			applyPositional(fs, cfg)

			if cfg.Content != tc.content {
				t.Errorf("content = %q, want %q", cfg.Content, tc.content)
			}
			if cfg.Out != tc.out {
				t.Errorf("out = %q, want %q", cfg.Out, tc.out)
			}
		})
	}
}

// reorderArgs exists so positional paths and flags can be mixed. A flag whose
// value happens to look like a path, or a value that is itself a flag, has to
// survive it.
func TestReorderArgs(t *testing.T) {
	// The flag set is declared, because reorderArgs asks it which flags are
	// boolean rather than keeping a list, and a list drifted: --watch and
	// --multi-site were boolean and each ate the argument that followed it.
	newFlags := func() *flag.FlagSet {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.Bool("open", false, "")
		fs.Bool("multi-site", false, "")
		fs.Bool("watch", false, "")
		fs.Duration("watch-interval", time.Second, "")
		fs.String("out", "", "")
		fs.String("base", "", "")
		return fs
	}
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "already ordered", in: []string{"--out", "1", "--base", "2"}, want: []string{"--out", "1", "--base", "2"}},
		{name: "positional first", in: []string{"dir", "--out", "o"}, want: []string{"--out", "o", "dir"}},
		{name: "positional in the middle", in: []string{"--out", "1", "dir", "--base", "2"}, want: []string{"--out", "1", "--base", "2", "dir"}},
		{name: "equals form", in: []string{"--out=o", "dir"}, want: []string{"--out=o", "dir"}},
		{name: "boolean takes no value", in: []string{"--open", "dir"}, want: []string{"--open", "dir"}},
		{name: "a boolean does not eat the next flag", in: []string{"--multi-site", "--base", "/"}, want: []string{"--multi-site", "--base", "/"}},
		{name: "a path that looks like a flag is a value", in: []string{"--out", "-weird", "dir"}, want: []string{"--out", "-weird", "dir"}},
		{name: "two positionals", in: []string{"c", "o", "--open"}, want: []string{"--open", "c", "o"}},
		{name: "a duration is a value, not a flag", in: []string{"--watch-interval", "2s", "dir"}, want: []string{"--watch-interval", "2s", "dir"}},
		{name: "empty", in: nil, want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reorderArgs(tc.in, newFlags())
			if len(got) != len(tc.want) {
				t.Fatalf("reorderArgs(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("reorderArgs(%q) = %q, want %q", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestIsBoolFlag(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Bool("multi-site", false, "")
	fs.Bool("bundle", false, "")
	fs.Bool("watch", false, "")
	fs.Duration("watch-interval", time.Second, "")
	fs.String("base", "", "")

	for _, name := range []string{"--multi-site", "-multi-site", "--bundle", "--watch"} {
		if !isBoolFlag(fs, name) {
			t.Errorf("isBoolFlag(%q) = false, want true", name)
		}
	}
	if isBoolFlag(fs, "--base") {
		t.Error("isBoolFlag(--base) = true, but it takes a value")
	}
	if isBoolFlag(fs, "--watch-interval") {
		t.Error("isBoolFlag(--watch-interval) = true, but it takes a value")
	}
	// The help flags are answered by the flag package rather than declared.
	if !isBoolFlag(fs, "--help") || !isBoolFlag(fs, "-h") {
		t.Error("the help flags are not recognised as boolean")
	}
}

// TestEnvDuration covers the environment fallback for --watch-interval: a valid
// duration is taken, and an unset or unparseable value falls back to the default
// rather than failing the process.
func TestEnvDuration(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want time.Duration
	}{
		{name: "unset uses the default", set: false, want: time.Second},
		{name: "empty uses the default", set: true, val: "", want: time.Second},
		{name: "valid duration", set: true, val: "250ms", want: 250 * time.Millisecond},
		{name: "seconds", set: true, val: "5s", want: 5 * time.Second},
		{name: "unparseable uses the default", set: true, val: "soon", want: time.Second},
		{name: "a bare number is not a duration", set: true, val: "2", want: time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("OKF_WIKI_WATCH_INTERVAL", tc.val)
			} else {
				os.Unsetenv("OKF_WIKI_WATCH_INTERVAL")
			}
			if got := envDuration("OKF_WIKI_WATCH_INTERVAL", time.Second); got != tc.want {
				t.Errorf("envDuration = %v, want %v", got, tc.want)
			}
		})
	}
}

// envAllowEmpty is the one environment read where an empty value is data: an
// empty MCP prefix means "no segment", so it must not fall back to the default.
func TestEnvAllowEmpty(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want string
	}{
		{name: "unset uses the default", set: false, want: "_"},
		{name: "empty is a value", set: true, val: "", want: ""},
		{name: "a value is taken", set: true, val: "api", want: "api"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("OKF_WIKI_MCP_PREFIX", tc.val)
			} else {
				os.Unsetenv("OKF_WIKI_MCP_PREFIX")
			}
			if got := envAllowEmpty("OKF_WIKI_MCP_PREFIX", "_"); got != tc.want {
				t.Errorf("envAllowEmpty = %q, want %q", got, tc.want)
			}
		})
	}
}

// --lang is bound in bindCommon with the other flags, so its default, its
// environment variable and its precedence over that variable are the CLI's
// contract. The rendered page carries the value, which the wiki package's own
// tests cover.
func TestBindCommonLang(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "unset uses the default", flag: "", env: "", want: wiki.DefaultLang},
		{name: "flag sets the language", flag: "de", env: "", want: "de"},
		{name: "environment sets the language", flag: "", env: "fr", want: "fr"},
		{name: "flag beats the environment", flag: "de", env: "fr", want: "de"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKF_WIKI_LANG", tc.env)
			fs := flag.NewFlagSet("render", flag.ContinueOnError)
			fs.SetOutput(nil)
			cfg := bindCommon(fs)
			if tc.flag != "" {
				if err := fs.Parse([]string{"--lang", tc.flag}); err != nil {
					t.Fatal(err)
				}
			}
			if cfg.Lang != tc.want {
				t.Errorf("Lang = %q, want %q", cfg.Lang, tc.want)
			}
		})
	}
}

// --bundle is off unless it is asked for, and the flag beats the environment
// the way every other bound flag does.
func TestBindCommonBundle(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{name: "unset is off", env: "", want: false},
		{name: "flag turns it on", args: []string{"--bundle"}, env: "", want: true},
		{name: "explicit false beats the environment", args: []string{"--bundle=false"}, env: "true", want: false},
		{name: "environment turns it on", env: "true", want: true},
		{name: "environment stays off", env: "false", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OKF_WIKI_BUNDLE", tc.env)
			fs := flag.NewFlagSet("render", flag.ContinueOnError)
			fs.SetOutput(nil)
			cfg := bindCommon(fs)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			if cfg.Bundle != tc.want {
				t.Errorf("Bundle = %v, want %v", cfg.Bundle, tc.want)
			}
		})
	}
}

// TestReportTitleWarnings covers the render's warning output: the first pages
// are named in a stable order, the rest are counted, and the count reads as a
// plural or a singular.
func TestReportTitleWarnings(t *testing.T) {
	tests := []struct {
		name  string
		long  int
		count string
	}{
		{"counts the remainder", titleWarningShown + 2, "2 more pages have a title that cannot work as a name"},
		{"counts one page in the singular", titleWarningShown + 1, "1 more page has a title that cannot work as a name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			must := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			must("index.md", "---\ntitle: Home\n---\n# Home\n")
			for i := 0; i < tc.long; i++ {
				must(fmt.Sprintf("long%d.md", i), "---\ntitle: "+strings.Repeat("x", 81)+"\n---\n# Long\n")
			}
			r, err := wiki.New(wiki.Config{Content: dir, Base: "/", Brand: wiki.Brand{Name: "t"}})
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			reportTitleWarnings(&out, r)

			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != titleWarningShown+1 {
				t.Fatalf("report has %d lines, want %d:\n%s", len(lines), titleWarningShown+1, out.String())
			}
			if !strings.Contains(lines[0], "okf-wiki: warning: page long0 has a 81-character title; keep it to 80 or fewer") {
				t.Errorf("first line = %q, want the first page named with what to do", lines[0])
			}
			if want := "okf-wiki: warning: " + tc.count; lines[len(lines)-1] != want {
				t.Errorf("last line = %q, want %q", lines[len(lines)-1], want)
			}
		})
	}
}
