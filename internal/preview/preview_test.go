package preview

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractEmbeddedJPEGChoosesLargestPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.arw")

	small := encodeJPEG(t, 10, 8)
	large := encodeJPEG(t, 24, 16)
	data := bytes.Join([][]byte{
		[]byte("raw-prefix"),
		small,
		[]byte("raw-middle"),
		large,
		[]byte("raw-suffix"),
	}, nil)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ExtractEmbeddedJPEG(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 24 || cfg.Height != 16 {
		t.Fatalf("expected largest preview 24x16, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestScaleKeepsAspectRatio(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	scaled := Scale(img, 100)
	if scaled.Bounds().Dx() != 100 || scaled.Bounds().Dy() != 50 {
		t.Fatalf("expected 100x50, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}
}

func encodeJPEG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
