package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func changesHealthBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
---
# Home

[Broken](missing.md)
[Cycle A](a.md)
`)
	mustWrite(t, filepath.Join(dir, "a.md"), `---
title: A
---
# A

[Back](b.md)
`)
	mustWrite(t, filepath.Join(dir, "b.md"), `---
title: B
---
# B

[Back](a.md)
`)
	mustWrite(t, filepath.Join(dir, "orphan.md"), `---
title: Orphan
---
# Orphan
`)
	mustWrite(t, filepath.Join(dir, "tag-only.md"), `---
title: Tag only
tags: [solo]
---
# Tag only
`)
	mustWrite(t, filepath.Join(dir, "stale.md"), `---
title: Stale
stale_after: "2020-01-01T00:00:00Z"
---
# Stale
`)
	return dir
}

func TestChangesAndHealth(t *testing.T) {
	dir := changesHealthBundle(t)
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	changes, err := LoadAgentChanges(site)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) == 0 || changes.RenderID == "" {
		t.Errorf("first changes = %+v, want added pages", changes)
	}
	for _, c := range changes.Changes {
		if c.Kind != "added" {
			t.Errorf("first render change = %+v, want added", c)
		}
	}
	health, err := LoadGraphHealth(site)
	if err != nil {
		t.Fatal(err)
	}
	if len(health.BrokenLinks) != 1 || health.BrokenLinks[0].To != "page:missing" {
		t.Errorf("broken links = %+v", health.BrokenLinks)
	}
	if !hasHealthPage(health.Orphans, "page:orphan") || !hasHealthPage(health.Orphans, "page:tag-only") {
		t.Errorf("orphans = %+v", health.Orphans)
	}
	if hasHealthPage(health.Orphans, "page:index") {
		t.Error("the home page was reported as an orphan")
	}
	if !hasHealthPage(health.TagOnly, "page:tag-only") {
		t.Errorf("tag-only pages = %+v", health.TagOnly)
	}
	if len(health.Cycles) != 1 || len(health.Cycles[0]) != 2 {
		t.Errorf("cycles = %+v", health.Cycles)
	}
	if !hasHealthPage(health.Stale, "page:stale") {
		t.Errorf("stale pages = %+v", health.Stale)
	}
	if len(health.MostConnected) == 0 {
		t.Error("health has no connected pages")
	}

	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home changed
---
# Home changed

[Broken](missing.md)
[Cycle A](a.md)
`)
	r, err = New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	changed, err := LoadAgentChanges(site)
	if err != nil {
		t.Fatal(err)
	}
	foundChanged := false
	for _, c := range changed.Changes {
		if c.ID == "page:index" && c.Kind == "changed" {
			foundChanged = true
		}
	}
	if !foundChanged || changed.PreviousRenderID == "" {
		t.Errorf("second changes = %+v, want index changed and a previous render", changed)
	}

	if err := os.Remove(filepath.Join(dir, "orphan.md")); err != nil {
		t.Fatal(err)
	}
	r, err = New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	removed, err := LoadAgentChanges(site)
	if err != nil {
		t.Fatal(err)
	}
	foundRemoved := false
	for _, c := range removed.Changes {
		if c.ID == "page:orphan" && c.Kind == "removed" {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Errorf("third changes = %+v, want orphan removed", removed)
	}
}

func hasHealthPage(pages []HealthPage, id string) bool {
	for _, p := range pages {
		if p.ID == id {
			return true
		}
	}
	return false
}

// TestTreeChangesUseNamespacedIDs covers the aggregate state at a tree base.
func TestTreeChangesUseNamespacedIDs(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\ntitle: "+strings.ToUpper(wiki[:1])+wiki[1:]+"\n---\n# Home\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	changes, err := LoadAgentChanges(filepath.Join(out, "family"))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 2 {
		t.Fatalf("tree changes = %+v, want two added pages", changes)
	}
	for _, c := range changes.Changes {
		if !strings.Contains(c.ID, ":page:index") {
			t.Errorf("tree change ID is not namespaced: %q", c.ID)
		}
	}
	health, err := LoadGraphHealth(filepath.Join(out, "family"))
	if err != nil {
		t.Fatal(err)
	}
	if health.RenderID != changes.RenderID {
		t.Errorf("health render ID = %q, changes = %q", health.RenderID, changes.RenderID)
	}
	for _, orphan := range health.Orphans {
		if strings.HasSuffix(orphan.ID, ":page:index") {
			t.Errorf("tree index page was reported as an orphan: %+v", orphan)
		}
	}
}

// TestMostConnectedIgnoresPageOrderEdges keeps typed navigation from making an
// unlinked island look like the best starting point.
func TestMostConnectedIgnoresPageOrderEdges(t *testing.T) {
	pages := []AgentPage{{ID: "page:a", Title: "A"}, {ID: "page:b", Title: "B"}, {ID: "page:c", Title: "C"}}
	graph := GraphIndex{
		Version: graphVersion,
		Nodes: []GraphNode{
			{ID: "page:a", Title: "A", Kind: "page"},
			{ID: "page:b", Title: "B", Kind: "page"},
			{ID: "page:c", Title: "C", Kind: "page"},
		},
		Edges: []GraphEdge{
			{From: "page:a", To: "page:b", Kind: "next"},
			{From: "page:b", To: "page:a", Kind: "previous"},
			{From: "page:b", To: "page:c", Kind: "next"},
			{From: "page:c", To: "page:b", Kind: "previous"},
		},
	}
	health := buildGraphHealth(graph, pages, "2026-10-24")
	if len(health.MostConnected) != 0 {
		t.Errorf("page-order edges produced most-connected pages: %+v", health.MostConnected)
	}
}
