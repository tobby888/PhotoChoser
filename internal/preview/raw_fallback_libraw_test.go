//go:build cgo && libraw

package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibRawFallbackRejectsNonRAWFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-raw.ARW")
	if err := os.WriteFile(path, []byte("definitely not raw image data"), 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := loadRAWFallback(path)
	if err == nil {
		t.Fatalf("loadRAWFallback succeeded on non-RAW data: %v", img.Bounds())
	}
	if !strings.HasPrefix(err.Error(), "libraw decode failed:") {
		t.Fatalf("expected LibRaw decode error, got %v", err)
	}
}
