package preview

import (
	"bytes"
	"encoding/binary"
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

func TestLoadAppliesJPEGOrientation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portrait.jpg")
	if err := os.WriteFile(path, addEXIFOrientation(encodeJPEG(t, 2, 3), 6), 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected rotated image 3x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestLoadAppliesRawTIFFOrientationToEmbeddedJPEG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portrait.arw")
	data := bytes.Join([][]byte{
		tiffHeaderWithOrientation(8),
		[]byte("raw-data"),
		encodeJPEG(t, 2, 3),
	}, nil)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected rotated embedded preview 3x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestApplyOrientationRotatesClockwise(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	img.Set(0, 0, red)
	img.Set(1, 2, blue)

	rotated := ApplyOrientation(img, 6)
	if rotated.Bounds().Dx() != 3 || rotated.Bounds().Dy() != 2 {
		t.Fatalf("expected rotated bounds 3x2, got %dx%d", rotated.Bounds().Dx(), rotated.Bounds().Dy())
	}
	if got := color.RGBAModel.Convert(rotated.At(2, 0)); got != red {
		t.Fatalf("expected top-left source pixel at (2,0), got %#v", got)
	}
	if got := color.RGBAModel.Convert(rotated.At(0, 1)); got != blue {
		t.Fatalf("expected bottom-right source pixel at (0,1), got %#v", got)
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

func addEXIFOrientation(jpegBytes []byte, orientation uint16) []byte {
	exif := []byte("Exif\x00\x00")
	tiff := make([]byte, 8+2+12+4)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	entry := tiff[10:22]
	binary.LittleEndian.PutUint16(entry[0:2], 0x0112)
	binary.LittleEndian.PutUint16(entry[2:4], 3)
	binary.LittleEndian.PutUint32(entry[4:8], 1)
	binary.LittleEndian.PutUint16(entry[8:10], orientation)
	exif = append(exif, tiff...)

	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:4], uint16(len(exif)+2))
	segment = append(segment, exif...)

	out := append([]byte{}, jpegBytes[:2]...)
	out = append(out, segment...)
	out = append(out, jpegBytes[2:]...)
	return out
}

func tiffHeaderWithOrientation(orientation uint16) []byte {
	tiff := make([]byte, 8+2+12+4)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	entry := tiff[10:22]
	binary.LittleEndian.PutUint16(entry[0:2], 0x0112)
	binary.LittleEndian.PutUint16(entry[2:4], 3)
	binary.LittleEndian.PutUint32(entry[4:8], 1)
	binary.LittleEndian.PutUint16(entry[8:10], orientation)
	return tiff
}
