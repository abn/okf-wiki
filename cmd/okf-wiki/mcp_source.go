package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abn/okf-wiki/internal/wiki"
)

// mcpSource is one configured upstream for the standalone federator.
type mcpSource struct {
	name     string
	base     string
	basePath string
	read     wiki.SourceReader
}

type sourceFlagList []string

func (s *sourceFlagList) String() string { return strings.Join(*s, ",") }

func (s *sourceFlagList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type tokenFlagMap map[string]string

func (t *tokenFlagMap) String() string { return "" }

func (t *tokenFlagMap) Set(value string) error {
	name, envName, ok := strings.Cut(value, "=")
	if !ok || name == "" || envName == "" {
		return fmt.Errorf("source token must be NAME=ENV")
	}
	(*t)[name] = envName
	return nil
}

// buildMCPSources turns repeated --source values into readers. With no
// --source it keeps the historical single local output source.
func buildMCPSources(out, base string, sources []string, tokens map[string]string) ([]mcpSource, time.Duration, error) {
	if len(sources) == 0 {
		siteDir, err := (wiki.Config{Out: out, Base: base}).SiteDir(out)
		if err != nil {
			return nil, 0, err
		}
		read := func(name string) ([]byte, error) {
			return os.ReadFile(filepath.Join(siteDir, name))
		}
		localBase := strings.TrimSpace(base)
		if !strings.HasPrefix(localBase, "/") {
			localBase = "/" + localBase
		}
		if !strings.HasSuffix(localBase, "/") {
			localBase += "/"
		}
		return []mcpSource{{name: "local", base: localBase, basePath: localBase, read: read}}, 0, nil
	}
	seen := make(map[string]bool)
	var specs []mcpSource
	for _, raw := range sources {
		raw = strings.TrimSpace(raw)
		name, sourceBase, ok := strings.Cut(raw, "=")
		if !ok || name == "" || sourceBase == "" {
			return nil, 0, fmt.Errorf("source must be NAME=BASE_URL, got %q", raw)
		}
		if strings.ContainsAny(name, ": ,\t\n") {
			return nil, 0, fmt.Errorf("source name %q contains an unsafe character", name)
		}
		if seen[name] {
			return nil, 0, fmt.Errorf("duplicate source name %q", name)
		}
		seen[name] = true
		token := ""
		if envName := tokens[name]; envName != "" {
			token = os.Getenv(envName)
			if token == "" {
				return nil, 0, fmt.Errorf("source %s: environment variable %s is empty", name, envName)
			}
			if strings.HasPrefix(strings.ToLower(sourceBase), "http://") {
				fmt.Fprintf(os.Stderr, "okf-wiki: warning: source %s sends a bearer token over HTTP\n", name)
			}
		}
		https, err := wiki.NewHTTPSource(sourceBase, token)
		if err != nil {
			return nil, 0, fmt.Errorf("source %s: %w", name, err)
		}
		specs = append(specs, mcpSource{name: name, base: https.BaseURL(), basePath: https.BasePath(), read: https.Read})
	}
	for name := range tokens {
		if !seen[name] {
			return nil, 0, fmt.Errorf("source token names unknown source %q", name)
		}
	}
	return specs, 30 * time.Second, nil
}
