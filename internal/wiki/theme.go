package wiki

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// defaultThemeFS holds the theme that ships inside the binary. It is the
// fallback layer for every other theme, so a theme only restates what it
// actually changes.
//
//go:embed all:themes/default
var defaultThemeFS embed.FS

const defaultThemeRoot = "themes/default"

// Asset resolution modes. A theme declares, per asset, whether it takes the
// base layer, supplies its own, or removes the file entirely.
const (
	modeInherit = "inherit" // take the base layer's file (the default)
	modeReplace = "replace" // use this theme's file
	modeDrop    = "drop"    // emit no file for this key
)

// themeAsset is one entry of a manifest's "assets" map.
type themeAsset struct {
	Mode string `json:"mode"`
}

// themeManifest is a theme's theme.json. Every field is optional: a manifest
// with an empty "assets" map inherits the base layer wholesale, which is the
// smallest useful theme there is.
type themeManifest struct {
	Name        string                `json:"name"`
	Version     string                `json:"version"`
	Description string                `json:"description"`
	Template    string                `json:"template"`
	Slots       map[string]string     `json:"slots"`
	Assets      map[string]themeAsset `json:"assets"`
}

// themeFile is one resolved output file, named relative to the rendered wiki
// directory.
type themeFile struct {
	Path string
	Data []byte
}

// Theme is a fully resolved theme: a base layer with an optional override
// applied. Resolution happens once, at construction, so a render never reads a
// theme path and never resolves outside the theme directory.
type Theme struct {
	Name        string
	Version     string
	Description string
	Source      string // "embedded", or the directory an override was read from

	files        []themeFile
	templateText string
	slotText     map[string]string
	tmpl         *template.Template

	// assetVersion is the ?v= query on every asset URL, so a re-render or a
	// theme edit busts the browser cache.
	assetVersion string
}

// loadedLayer is the un-resolved contribution of one theme directory: the
// identity it claims, plus the files, template and slots it supplies.
type loadedLayer struct {
	Name        string
	Version     string
	Description string

	replace map[string][]themeFile // asset key -> files
	drop    []string               // asset keys to remove
	tmpl    string
	slots   map[string]string
}

// ResolveTheme returns the theme to render with. An empty dir yields the
// embedded default; a non-empty dir must hold a theme.json and is layered over
// that default, so an override only restates what it changes.
func ResolveTheme(dir string) (*Theme, error) {
	base, err := readTheme(defaultThemeFS, defaultThemeRoot)
	if err != nil {
		return nil, fmt.Errorf("embedded default theme: %w", err)
	}
	t := &Theme{
		Name:        base.Name,
		Version:     base.Version,
		Description: base.Description,
		Source:      "embedded",
	}
	if dir == "" {
		return t.compile(base)
	}

	st, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("theme %s: %w", dir, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("theme %s: not a directory", dir)
	}
	over, err := readTheme(os.DirFS(dir), ".")
	if err != nil {
		return nil, fmt.Errorf("theme %s: %w", dir, err)
	}
	// An override that does not name itself keeps the base layer's identity, so
	// a recolour-only theme still reports which design it descends from.
	t.Name = firstNonEmpty(over.Name, base.Name)
	t.Version = firstNonEmpty(over.Version, base.Version)
	t.Description = over.Description
	t.Source = dir
	return t.compile(base, over)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// readTheme reads a manifest and everything it points at from fsys. It resolves
// nothing against a base layer; that is compile's job.
func readTheme(fsys fs.FS, root string) (*loadedLayer, error) {
	raw, err := fs.ReadFile(fsys, path.Join(root, "theme.json"))
	if err != nil {
		return nil, fmt.Errorf("read theme.json: %w", err)
	}
	var m themeManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse theme.json: %w", err)
	}

	l := &loadedLayer{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		replace:     map[string][]themeFile{},
		slots:       map[string]string{},
	}

	keys := make([]string, 0, len(m.Assets))
	for k := range m.Assets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		mode := m.Assets[key].Mode
		if mode == "" {
			mode = modeInherit
		}
		switch mode {
		case modeInherit:
			// Nothing to read: the base layer keeps the file.
		case modeDrop:
			l.drop = append(l.drop, key)
		case modeReplace:
			files, err := readAsset(fsys, root, key)
			if err != nil {
				return nil, fmt.Errorf("asset %s: %w", key, err)
			}
			l.replace[key] = files
		default:
			return nil, fmt.Errorf("asset %s: unknown mode %q", key, mode)
		}
	}

	if m.Template != "" {
		b, err := fs.ReadFile(fsys, path.Join(root, m.Template))
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", m.Template, err)
		}
		l.tmpl = string(b)
	}
	slotNames := make([]string, 0, len(m.Slots))
	for name := range m.Slots {
		slotNames = append(slotNames, name)
	}
	sort.Strings(slotNames)
	for _, name := range slotNames {
		b, err := fs.ReadFile(fsys, path.Join(root, m.Slots[name]))
		if err != nil {
			return nil, fmt.Errorf("slot %s: %w", name, err)
		}
		l.slots[name] = string(b)
	}
	return l, nil
}

// readAsset collects the files an asset key contributes. A key ending in "/"
// names a subtree, so "fonts/" is replaced without listing every file.
func readAsset(fsys fs.FS, root, key string) ([]themeFile, error) {
	name := path.Clean(key)
	dir := path.Join(root, "assets", name)

	st, err := fs.Stat(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("no assets/%s in theme: %w", name, err)
	}
	if !st.IsDir() {
		b, err := fs.ReadFile(fsys, dir)
		if err != nil {
			return nil, err
		}
		return []themeFile{{Path: name, Data: b}}, nil
	}

	var out []themeFile
	err = fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out = append(out, themeFile{Path: name + "/" + filepath.ToSlash(rel), Data: b})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("assets/%s is an empty directory", name)
	}
	return out, nil
}

// under reports whether an output path belongs to an asset key, matching the
// key itself and anything below it. A key that names a directory therefore
// replaces that whole directory, which is what a subtree key means. A trailing
// slash on the key makes no difference: path.Clean drops it.
func under(p, key string) bool {
	clean := path.Clean(key)
	return p == clean || strings.HasPrefix(p, clean+"/")
}

// resolve merges the base layer with the override layers, in order, into the
// final file set.
func (t *Theme) resolve(layers ...*loadedLayer) {
	files := map[string][]byte{}
	for _, l := range layers {
		for _, key := range l.drop {
			for p := range files {
				if under(p, key) {
					delete(files, p)
				}
			}
		}
		for key, got := range l.replace {
			for p := range files {
				if under(p, key) {
					delete(files, p)
				}
			}
			for _, f := range got {
				files[f.Path] = f.Data
			}
		}
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	t.files = t.files[:0]
	for _, p := range paths {
		t.files = append(t.files, themeFile{Path: p, Data: files[p]})
	}
}

// compile resolves the layers, then parses the template and fingerprints the
// result. Layers are listed base-first, so each one is applied over the ones
// before it and the last non-empty template or slot wins. That single ordering
// is what makes an override win, whether it replaces a whole asset key, a
// subtree, or just the shell.
func (t *Theme) compile(layers ...*loadedLayer) (*Theme, error) {
	if len(layers) == 0 {
		return nil, fmt.Errorf("theme %s: nothing to compile", t.Source)
	}
	t.resolve(layers...)

	t.templateText = ""
	t.slotText = map[string]string{}
	for _, l := range layers {
		if l == nil {
			continue
		}
		if l.tmpl != "" {
			t.templateText = l.tmpl
		}
		for name, text := range l.slots {
			t.slotText[name] = text
		}
	}
	if t.templateText == "" {
		return nil, fmt.Errorf("theme %s: no template declared in theme.json", t.Source)
	}

	tmpl, err := template.New("shell").Parse(t.templateText)
	if err != nil {
		// A template that does not parse is a configuration error, so it fails
		// the render. A template that parses but fails at execute time is not
		// caught here: RenderPage reports it in the page body instead.
		return nil, fmt.Errorf("theme %s: parse template: %w", t.Source, err)
	}
	t.tmpl = tmpl
	t.assetVersion = t.fingerprint()
	return t, nil
}

// fingerprint hashes every resolved byte, the template, and the slots, so any
// change to the effective theme produces a new asset version. Only the first
// 10 hex characters are kept: the value is a cache-buster, not an integrity
// check.
func (t *Theme) fingerprint() string {
	h := sha256.New()
	for _, f := range t.files {
		fmt.Fprintf(h, "%s\x00%d\x00", f.Path, len(f.Data))
		h.Write(f.Data)
	}
	fmt.Fprintf(h, "template\x00%d\x00", len(t.templateText))
	h.Write([]byte(t.templateText))
	slots := make([]string, 0, len(t.slotText))
	for name := range t.slotText {
		slots = append(slots, name)
	}
	sort.Strings(slots)
	for _, name := range slots {
		fmt.Fprintf(h, "slot:%s\x00%d\x00", name, len(t.slotText[name]))
		h.Write([]byte(t.slotText[name]))
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func (t *Theme) AssetVersion() string { return t.assetVersion }

func (t *Theme) Template() *template.Template { return t.tmpl }

// Slot returns a named slot's text, trimmed, or empty when unset. The text is
// template.HTML because a slot is the theme's own markup, not page content.
func (t *Theme) Slot(name string) template.HTML {
	return template.HTML(strings.TrimSpace(t.slotText[name]))
}

// SlotNames returns the theme's declared slot names, sorted.
func (t *Theme) SlotNames() []string {
	names := make([]string, 0, len(t.slotText))
	for name := range t.slotText {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// WriteTo emits the resolved theme into dest, which is the rendered wiki
// directory itself rather than an output root.
func (t *Theme) WriteTo(dest string) error {
	for _, f := range t.files {
		target := filepath.Join(dest, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, f.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
