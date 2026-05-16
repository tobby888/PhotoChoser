package main

import (
	"testing"

	"photochoser/internal/photos"
)

func TestMergeScanProgressPreservesSelectedPhotos(t *testing.T) {
	existing := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW", Selected: true},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
	}
	scanned := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
		{Path: "/card/DCIM/c.ARW", Name: "c.ARW"},
	}

	merged := mergeScanProgress(scanned, existing)
	if !merged[0].Selected {
		t.Fatal("expected previously selected photo to remain selected")
	}
	if merged[1].Selected || merged[2].Selected {
		t.Fatal("expected unselected and new photos to remain unselected")
	}
}

func TestCountNewItems(t *testing.T) {
	existing := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
	}
	scanned := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
		{Path: "/card/DCIM/c.ARW", Name: "c.ARW"},
	}

	if got := countNewItems(scanned, existing); got != 2 {
		t.Fatalf("expected 2 new photos, got %d", got)
	}
}

func TestPreferredItemIDKeepsCurrentPath(t *testing.T) {
	items := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
	}

	if got := preferredItemID(items, "/card/DCIM/b.ARW"); got != 1 {
		t.Fatalf("expected preferred item id 1, got %d", got)
	}
	if got := preferredItemID(items, "/card/DCIM/missing.ARW"); got != 0 {
		t.Fatalf("expected missing preferred path to fall back to 0, got %d", got)
	}
}
