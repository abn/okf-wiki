package wiki

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readBundleArchive opens a rendered archive and returns its entries by name.
func readBundleArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open archive %s: %v", path, err)
	}
	defer zr.Close()
	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s in archive: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s from archive: %v", f.Name, err)
		}
		entries[f.Name] = body
	}
	return entries
}

// TestBundleArchive covers the downloaded bundle: the source files under one
// top-level folder, with frontmatter, assets and paths intact, and no rendered
// page in place of them.
func TestBundleArchive(t *testing.T) {
	content := docsDir(t)
	out := t.TempDir()
	r, err := New(Config{Content: content, Base: "/wiki/", Brand: Brand{Name: "okf-wiki"}, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	site := filepath.Join(out, "wiki")
	entries := readBundleArchive(t, filepath.Join(site, bundleName))
	if len(entries) == 0 {
		t.Fatal("the archive is empty")
	}
	for name := range entries {
		if !strings.HasPrefix(name, "docs/") {
			t.Errorf("archive entry %q is outside the bundle folder", name)
		}
	}
	if _, ok := entries["docs/index.html"]; ok {
		t.Error("the archive holds a rendered page instead of the source bundle")
	}

	source, err := os.ReadFile(filepath.Join(content, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	index := entries["docs/index.md"]
	if string(index) != string(source) {
		t.Error("index.md in the archive differs from the source bundle")
	}
	if !strings.HasPrefix(string(index), "---\nokf_version:") {
		t.Errorf("index.md lost its frontmatter:\n%s", index)
	}

	asset, err := os.ReadFile(filepath.Join(content, "usage", "assets", "multi-site-cards.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := entries["docs/usage/assets/multi-site-cards.png"]; string(got) != string(asset) {
		t.Error("a binary asset in the archive differs from the source bundle")
	}

	if got := entries["docs/design/rendering.md"]; len(got) == 0 {
		t.Error("a nested Markdown file is missing from the archive")
	}

	files, err := markdownFiles(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		rel, err := filepath.Rel(content, f)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := entries["docs/"+filepath.ToSlash(rel)]; !ok {
			t.Errorf("Markdown file %s is missing from the archive", rel)
		}
	}
}

// TestBundleArchiveSkipsDotfiles covers the one exclusion: a working tree's
// .git or .env is not part of the bundle, so it is not part of the download.
func TestBundleArchiveSkipsDotfiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "notes.md"), "# Notes\n")
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustWrite(t, filepath.Join(dir, ".git", "config"), "[core]\n")
	mustWrite(t, filepath.Join(dir, ".env"), "TOKEN=secret\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	entries := readBundleArchive(t, filepath.Join(out, "wiki", bundleName))
	if _, ok := entries[filepath.Base(dir)+"/notes.md"]; !ok {
		t.Fatal("the archive is missing a bundle file")
	}
	for name := range entries {
		for _, part := range strings.Split(name, "/") {
			if strings.HasPrefix(part, ".") {
				t.Errorf("archive entry %q comes from a dotfile or dot-directory", name)
			}
		}
	}
}

// TestBundleDownloadControl covers the page control: every page of a wiki
// offers that wiki's archive, not the site's chrome.
func TestBundleDownloadControl(t *testing.T) {
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), Base: "/wiki/", Brand: Brand{Name: "okf-wiki"}, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	site := filepath.Join(out, "wiki")
	for _, page := range []string{
		filepath.Join(site, "index.html"),
		filepath.Join(site, "design", "rendering.html"),
		filepath.Join(site, "tags", "design.html"),
	} {
		body, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `class="icon-btn bundle-btn" href="/wiki/okf-bundle.zip" download`) {
			t.Errorf("%s does not offer the whole bundle for download", filepath.Base(page))
		}
		if !strings.Contains(string(body), `class="side-download"><a href="/wiki/okf-bundle.zip" download`) {
			t.Errorf("%s has no drawer entry for the bundle", filepath.Base(page))
		}
	}
}

// TestBundleOffline covers the archive in an offline folder: the control is
// relative, and the from-disk page still names a file that exists.
func TestBundleOffline(t *testing.T) {
	content := offlineBundle(t)
	out := t.TempDir()
	r, err := New(Config{Content: content, Base: "/wiki/", Brand: Brand{Name: "okf-wiki"}, Offline: true, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	body, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `class="icon-btn bundle-btn" href="./okf-bundle.zip" download`) {
		t.Error("the offline page does not offer the bundle at a relative URL")
	}
	if _, err := os.Stat(filepath.Join(site, bundleName)); err != nil {
		t.Fatalf("the offline archive is missing: %v", err)
	}
}

// TestBundleMultiSite covers one archive per wiki of a tree: each wiki's pages
// link to their own, and a group page, which has no bundle, carries no control.
func TestBundleMultiSite(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, name))
		mustWrite(t, filepath.Join(dir, name, "index.md"),
			"---\ntitle: "+strings.ToUpper(name[:1])+name[1:]+"\n---\n# Home\n")
	}

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"alpha", "beta"} {
		archive := filepath.Join(out, "family", name, bundleName)
		entries := readBundleArchive(t, archive)
		if _, ok := entries[name+"/index.md"]; !ok {
			t.Errorf("%s archive does not hold its own bundle at the top level", name)
		}
		body, err := os.ReadFile(filepath.Join(out, "family", name, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `class="icon-btn bundle-btn" href="/family/`+name+`/okf-bundle.zip" download`) {
			t.Errorf("%s links to an archive other than its own", name)
		}
	}

	for _, page := range []string{
		filepath.Join(out, "family", "index.html"),
		filepath.Join(out, "family", "graph.html"),
	} {
		body, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "bundle-btn") || strings.Contains(string(body), "side-download") {
			t.Errorf("%s offers a bundle it does not have", filepath.Base(page))
		}
	}
}

// TestBundleFolder covers the archive's top-level name: the content path as
// given, and a fallback for a path that names no directory of its own or whose
// base would climb out of the extraction directory.
func TestBundleFolder(t *testing.T) {
	for content, want := range map[string]string{
		"docs":              "docs",
		"./docs/":           "docs",
		"/tmp/example/wiki": "wiki",
		".":                 "okf-bundle",
		"..":                "okf-bundle",
		"/":                 "okf-bundle",
	} {
		if got := bundleFolder(content); got != want {
			t.Errorf("bundleFolder(%q) = %q, want %q", content, got, want)
		}
	}
}

// TestBundleCollision covers the reserved path. A bundle that already holds
// okf-bundle.zip would lose that file to the export, so the render fails and
// names it rather than overwriting it.
func TestBundleCollision(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, bundleName), "not ours\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	err = r.RenderAll(t.TempDir())
	if err == nil {
		t.Fatal("a bundle that occupies the archive path rendered without an error")
	}
	if !errors.Is(err, errBundleCollision) {
		t.Fatalf("error is not a bundle collision: %v", err)
	}
	if !strings.Contains(err.Error(), bundleName) {
		t.Errorf("the error does not name the occupied file: %v", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("the error does not name the bundle root: %v", err)
	}
}

// TestBundleTreeCollision covers the same collision in a tree: the wiki's name
// has to reach the operator, and the partial tree must not be published with
// that wiki silently missing.
func TestBundleTreeCollision(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, name))
		mustWrite(t, filepath.Join(dir, name, "index.md"), "---\ntitle: "+name+"\n---\n# Home\n")
	}
	mustWrite(t, filepath.Join(dir, "beta", bundleName), "not ours\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err == nil {
		t.Fatal("a wiki that occupies the archive path rendered the tree without an error")
	}
	if !errors.Is(err, errBundleCollision) {
		t.Fatalf("error is not a bundle collision: %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "beta")) {
		t.Errorf("the error does not name the wiki: %v", err)
	}
	if wikis != 0 {
		t.Errorf("rendered %d wikis around a fatal collision, want 0", wikis)
	}
	if _, err := os.Stat(filepath.Join(out, "family", "alpha", "index.html")); !os.IsNotExist(err) {
		t.Error("a failed tree render published a wiki")
	}
}

// TestBundleRootWikiTree covers a tree whose root is itself a wiki: its content
// root is the whole tree, so one archive holds every wiki under it.
func TestBundleRootWikiTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Root\n---\n# Root\n")
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"), "---\ntitle: Alpha\n---\n# Alpha\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	entries := readBundleArchive(t, filepath.Join(out, "family", bundleName))
	folder := filepath.Base(dir)
	for _, name := range []string{folder + "/index.md", folder + "/alpha/index.md"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("the root wiki's archive is missing %s", name)
		}
	}
	body, err := os.ReadFile(filepath.Join(out, "family", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `class="icon-btn bundle-btn" href="/family/okf-bundle.zip"`) {
		t.Error("the tree's graph view does not offer the root wiki's archive")
	}
}

// TestBundleOffByDefault covers the opt-in: a render without --bundle writes no
// archive and offers no control, and a source file that happens to be named
// okf-bundle.zip is an ordinary asset rather than a reserved path.
func TestBundleOffByDefault(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, bundleName), "a reader's file\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatalf("a render without --bundle failed: %v", err)
	}
	site := filepath.Join(out, "wiki")
	body, err := os.ReadFile(filepath.Join(site, bundleName))
	if err != nil {
		t.Fatalf("the bundle's own file was not copied: %v", err)
	}
	if string(body) != "a reader's file\n" {
		t.Error("a render without --bundle replaced the bundle's own file")
	}
	page, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "bundle-btn") || strings.Contains(string(page), "side-download") {
		t.Error("a render without --bundle offered a download")
	}
}

// TestBundleMultiSiteOff covers the tree gate: with the flag off, a wiki's own
// file named okf-bundle.zip is copied and served, and no control is offered.
func TestBundleMultiSiteOff(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"), "---\ntitle: Alpha\n---\n# Alpha\n")
	mustWrite(t, filepath.Join(dir, "alpha", bundleName), "a reader's file\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatalf("a tree render without --bundle failed: %v", err)
	}
	site := filepath.Join(out, "family", "alpha")
	body, err := os.ReadFile(filepath.Join(site, bundleName))
	if err != nil {
		t.Fatalf("the wiki's own file was not copied: %v", err)
	}
	if string(body) != "a reader's file\n" {
		t.Error("a tree render without --bundle replaced the wiki's own file")
	}
	page, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "bundle-btn") || strings.Contains(string(page), "side-download") {
		t.Error("a tree render without --bundle offered a download")
	}
}
