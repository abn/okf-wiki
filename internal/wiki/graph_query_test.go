package wiki

import (
	"path/filepath"
	"testing"
)

func graphQueryFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
tags: [guide]
---
# Home

[Install](guide/install.md)
`)
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "install.md"), `---
title: Install
tags: [guide]
---
# Install

[Home](../index.md)
`)
	mustWrite(t, filepath.Join(dir, "reference.md"), `---
title: Reference
---
# Reference

[Home](index.md)
`)
	mustWrite(t, filepath.Join(dir, "orphan.md"), "---\ntitle: Orphan\n---\n# Orphan\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "wiki")
}

func TestGraphNeighborsAndPath(t *testing.T) {
	site := graphQueryFixture(t)
	c, err := LoadGraphCorpus(site)
	if err != nil {
		t.Fatal(err)
	}
	links, err := c.Neighbors("page:index", 1, []string{"link"})
	if err != nil {
		t.Fatal(err)
	}
	if len(links.Nodes) < 2 || !hasNode(links.Nodes, "page:guide/install") {
		t.Errorf("link neighbours = %+v", links)
	}
	for _, e := range links.Edges {
		if e.Kind != "link" {
			t.Errorf("link filter kept %+v", e)
		}
	}
	all, err := c.Neighbors("page:index", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEdge(all.Edges, "previous") && !hasEdge(all.Edges, "next") {
		t.Errorf("page order edges are missing: %+v", all.Edges)
	}

	path, err := c.Path("page:guide/install", "page:orphan", 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !path.Found || path.Length < 2 {
		t.Errorf("path = %+v, want a path to the orphan", path)
	}
	linkOnly, err := c.Path("page:guide/install", "page:orphan", 6, []string{"link"})
	if err != nil {
		t.Fatal(err)
	}
	if linkOnly.Found {
		t.Errorf("link-only path = %+v, want no path", linkOnly)
	}
}

func hasNode(nodes []GraphNodeInfo, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func hasEdge(edges []GraphEdgeInfo, kind string) bool {
	for _, e := range edges {
		if e.Kind == kind {
			return true
		}
	}
	return false
}
