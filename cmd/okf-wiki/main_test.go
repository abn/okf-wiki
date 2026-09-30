package main

import (
	"flag"
	"testing"

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
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "already ordered", in: []string{"--a", "1", "--b", "2"}, want: []string{"--a", "1", "--b", "2"}},
		{name: "positional first", in: []string{"dir", "--out", "o"}, want: []string{"--out", "o", "dir"}},
		{name: "positional in the middle", in: []string{"--a", "1", "dir", "--b", "2"}, want: []string{"--a", "1", "--b", "2", "dir"}},
		{name: "equals form", in: []string{"--out=o", "dir"}, want: []string{"--out=o", "dir"}},
		{name: "boolean takes no value", in: []string{"--open", "dir"}, want: []string{"--open", "dir"}},
		{name: "a path that looks like a flag is a value", in: []string{"--out", "-weird", "dir"}, want: []string{"--out", "-weird", "dir"}},
		{name: "two positionals", in: []string{"c", "o", "--open"}, want: []string{"--open", "c", "o"}},
		{name: "empty", in: nil, want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reorderArgs(tc.in, flag.NewFlagSet("t", flag.ContinueOnError))
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
