package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DetectContent finds an OKF bundle when --content was not supplied, which is
// what makes `podman run -v $(pwd)/docs <image>` work without naming a container
// path: podman mounts the host directory at its host path, and this walks the
// bind mounts looking for a directory that contains markdown. extra is prepended
// to the search and is how a caller overrides the conventional paths.
func DetectContent(extra []string) (string, error) {
	var candidates []string
	add := func(dirs ...string) { candidates = append(candidates, dirs...) }

	add(extra...)
	add("/content", "/docs", "/wiki-content")
	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}
	add(mountCandidates()...)

	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if isOKFBundle(abs) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("no OKF bundle found (looked in %s)", strings.Join(candidates, ", "))
}

// isOKFBundle reports whether dir looks like an OKF bundle: it holds markdown
// at its root or one level down. The test is deliberately loose, so a directory
// with a single Markdown file qualifies and a repository root can be picked
// over a nested docs/ directory.
func isOKFBundle(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if md, _ := filepath.Glob(filepath.Join(dir, "*.md")); len(md) > 0 {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if md, _ := filepath.Glob(filepath.Join(dir, e.Name(), "*.md")); len(md) > 0 {
			return true
		}
	}
	return false
}

// mountCandidates returns bind-mount points that could hold mounted content,
// skipping the container's own system mounts.
func mountCandidates() []string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mp := unescapeMount(fields[4])
		if isSystemPath(mp) {
			continue
		}
		out = append(out, mp)
	}
	sort.Strings(out)
	return out
}

func isSystemPath(p string) bool {
	if p == "/" {
		return true
	}
	for _, prefix := range []string{
		"/proc", "/sys", "/dev", "/run", "/etc", "/usr", "/lib", "/lib64",
		"/bin", "/sbin", "/boot", "/srv", "/var/lib",
	} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// unescapeMount decodes the octal escapes mountinfo uses for special bytes.
func unescapeMount(s string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(s)
}
