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

func TestSamePhotoPaths(t *testing.T) {
	items := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
	}

	if !samePhotoPaths(items, []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
	}) {
		t.Fatal("expected matching paths to be treated as unchanged")
	}
	if samePhotoPaths(items, []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/c.ARW", Name: "c.ARW"},
	}) {
		t.Fatal("expected changed paths to be detected")
	}
}

func TestThumbPreloadOrderStartsNearCurrentPhoto(t *testing.T) {
	got := thumbPreloadOrder(7, 3)
	want := []int{3, 4, 2, 5, 1, 6, 0}
	if len(got) != len(want) {
		t.Fatalf("expected %d ids, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected preload order at %d: got %#v want %#v", i, got, want)
		}
	}
}

func TestThumbPreloadOrderFallsBackToStart(t *testing.T) {
	got := thumbPreloadOrder(3, -1)
	want := []int{0, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected preload order: got %#v want %#v", got, want)
		}
	}
}

func TestThumbPriorityOrderLimitsToVisibleNeighborhood(t *testing.T) {
	got := thumbPriorityOrder(10, 5, 2)
	want := []int{5, 6, 4, 7, 3}
	if len(got) != len(want) {
		t.Fatalf("expected %d ids, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected priority order: got %#v want %#v", got, want)
		}
	}
}

func TestShouldSkipThumbJobWhileCurrentPreviewLoads(t *testing.T) {
	ui := &photoApp{}
	ui.scanToken.Store(12)
	ui.currentPreviewToken.Store(3)

	if !ui.shouldSkipThumbJob(thumbJob{id: 3, token: 12}) {
		t.Fatal("expected thumbnail work to pause while current preview is loading")
	}

	ui.currentPreviewToken.Store(0)
	if ui.shouldSkipThumbJob(thumbJob{id: 3, token: 12}) {
		t.Fatal("expected thumbnail work to resume after current preview finishes")
	}
}

func TestThumbLoadGateBlocksThumbnailsForSelectedPreview(t *testing.T) {
	ui := &photoApp{}
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		ui.thumbLoadGate.Lock()
		close(locked)
		<-release
		ui.thumbLoadGate.Unlock()
	}()
	<-locked

	go func() {
		ui.thumbLoadGate.RLock()
		ui.thumbLoadGate.RUnlock()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("expected selected preview lock to block thumbnail loading")
	default:
	}

	close(release)
	<-done
}

func TestPreferredItemIDKeepsCurrentPath(t *testing.T) {
	items := []photos.Photo{
		{Path: "/card/DCIM/a.ARW", Name: "a.ARW"},
		{Path: "/card/DCIM/b.ARW", Name: "b.ARW"},
	}

	if got := preferredItemID(items, "/card/DCIM/b.ARW", 0); got != 1 {
		t.Fatalf("expected preferred item id 1, got %d", got)
	}
	if got := preferredItemID(items, "/card/DCIM/missing.ARW", 1); got != 1 {
		t.Fatalf("expected missing preferred path to fall back to current index, got %d", got)
	}
	if got := preferredItemID(items, "/card/DCIM/missing.ARW", 9); got != 1 {
		t.Fatalf("expected oversized fallback to clamp to last item, got %d", got)
	}
}
