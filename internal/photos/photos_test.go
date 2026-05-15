package photos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsSupportedFilesRecursively(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.ARW"))
	writeFile(t, filepath.Join(dir, "._a.ARW"))
	writeFile(t, filepath.Join(nested, "b.CR3"))
	writeFile(t, filepath.Join(dir, "notes.txt"))

	items, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 photos, got %d", len(items))
	}
}

func TestMoveSelectedMovesPhotoAndXMP(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	raw := filepath.Join(srcDir, "event.NEF")
	xmp := filepath.Join(srcDir, "event.xmp")
	writeFile(t, raw)
	writeFile(t, xmp)

	moved, errs := MoveSelected([]Photo{{Path: raw, Name: "event.NEF", Selected: true}}, dstDir)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if moved != 1 {
		t.Fatalf("expected 1 moved photo, got %d", moved)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "event.NEF")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "event.xmp")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(raw); !os.IsNotExist(err) {
		t.Fatalf("expected source raw to be moved, stat err: %v", err)
	}
}

func TestFromPathsFiltersUnsupportedFilesAndDuplicates(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "event.ARW")
	jpg := filepath.Join(dir, "event.jpg")
	txt := filepath.Join(dir, "notes.txt")
	appleDouble := filepath.Join(dir, "._event.ARW")
	writeFile(t, raw)
	writeFile(t, jpg)
	writeFile(t, txt)
	writeFile(t, appleDouble)

	items := FromPaths([]string{txt, raw, raw, jpg, appleDouble})
	if len(items) != 2 {
		t.Fatalf("expected 2 imported photos, got %d", len(items))
	}
	if items[0].Name != "event.ARW" || items[1].Name != "event.jpg" {
		t.Fatalf("unexpected import order: %#v", items)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("photo"), 0o644); err != nil {
		t.Fatal(err)
	}
}
