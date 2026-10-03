package main

import (
	"flag"
	"os"
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
	fs.Bool("watch", false, "")
	fs.Duration("watch-interval", time.Second, "")
	fs.String("base", "", "")

	for _, name := range []string{"--multi-site", "-multi-site", "--watch"} {
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
