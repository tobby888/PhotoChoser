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

// tiffWithPreviews builds a little-endian TIFF whose IFD0 carries the
// orientation and a JPEGInterchangeFormat preview, and whose IFD1 carries a
// second preview, mirroring the Sony ARW layout.
func tiffWithPreviews(orientation uint16, ifd0JPEG []byte, ifd1JPEG []byte, rawData []byte) []byte {
	const ifd0Offset = 8
	ifd0Size := 2 + 3*12 + 4
	ifd1Offset := ifd0Offset + ifd0Size
	ifd1Size := 2 + 2*12 + 4
	jpeg0Offset := ifd1Offset + ifd1Size
	jpeg1Offset := jpeg0Offset + len(ifd0JPEG)

	out := make([]byte, jpeg0Offset)
	copy(out[:2], "II")
	binary.LittleEndian.PutUint16(out[2:4], 42)
	binary.LittleEndian.PutUint32(out[4:8], ifd0Offset)

	putEntry := func(at int, tag uint16, fieldType uint16, value uint32) {
		binary.LittleEndian.PutUint16(out[at:], tag)
		binary.LittleEndian.PutUint16(out[at+2:], fieldType)
		binary.LittleEndian.PutUint32(out[at+4:], 1)
		if fieldType == 3 {
			binary.LittleEndian.PutUint16(out[at+8:], uint16(value))
		} else {
			binary.LittleEndian.PutUint32(out[at+8:], value)
		}
	}

	binary.LittleEndian.PutUint16(out[ifd0Offset:], 3)
	putEntry(ifd0Offset+2, tagOrientation, 3, uint32(orientation))
	putEntry(ifd0Offset+14, tagJPEGOffset, 4, uint32(jpeg0Offset))
	putEntry(ifd0Offset+26, tagJPEGLength, 4, uint32(len(ifd0JPEG)))
	binary.LittleEndian.PutUint32(out[ifd0Offset+38:], uint32(ifd1Offset))

	binary.LittleEndian.PutUint16(out[ifd1Offset:], 2)
	putEntry(ifd1Offset+2, tagJPEGOffset, 4, uint32(jpeg1Offset))
	putEntry(ifd1Offset+14, tagJPEGLength, 4, uint32(len(ifd1JPEG)))

	out = append(out, ifd0JPEG...)
	out = append(out, ifd1JPEG...)
	return append(out, rawData...)
}

func TestReadTIFFPreviewUsesIFDPointers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.arw")
	// The raw data holds a larger JPEG that only a full scan would find; the
	// IFD-directed lookup must not need it.
	data := tiffWithPreviews(6, encodeJPEG(t, 80, 50), encodeJPEG(t, 16, 12), encodeJPEG(t, 400, 300))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	got, orientation, err := readTIFFPreview(file, 60)
	if err != nil {
		t.Fatal(err)
	}
	if orientation != 6 {
		t.Fatalf("expected IFD0 orientation 6, got %d", orientation)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 80 || cfg.Height != 50 {
		t.Fatalf("expected IFD0 preview 80x50, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestReadTIFFPreviewRejectsTooSmallPreviews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.arw")
	data := tiffWithPreviews(1, encodeJPEG(t, 30, 20), encodeJPEG(t, 16, 12), nil)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if _, _, err := readTIFFPreview(file, 60); err == nil {
		t.Fatal("expected previews below maxSide to fall back to the full scan")
	}
}

func TestLoadScaledRotatesTIFFPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portrait.arw")
	data := tiffWithPreviews(6, encodeJPEG(t, 80, 50), encodeJPEG(t, 16, 12), nil)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := LoadScaled(path, 40)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.RGBA); !ok {
		t.Fatalf("expected *image.RGBA for direct GPU upload, got %T", img)
	}
	if img.Bounds().Dx() != 25 || img.Bounds().Dy() != 40 {
		t.Fatalf("expected rotated 25x40 preview, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestReadTIFFPreviewReadsRAFHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.raf")
	preview := encodeJPEG(t, 80, 50)
	header := make([]byte, 100)
	copy(header, "FUJIFILMCCD-RAW ")
	binary.BigEndian.PutUint32(header[84:88], uint32(len(header)))
	binary.BigEndian.PutUint32(header[88:92], uint32(len(preview)))
	if err := os.WriteFile(path, append(header, preview...), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	got, _, err := readTIFFPreview(file, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, preview) {
		t.Fatal("expected RAF header preview bytes")
	}
}

func TestExtractEmbeddedJPEGHandlesMarkerAcrossReadChunks(t *testing.T) {
	preview := encodeJPEG(t, 18, 12)
	for _, split := range []int{1, 2, 3, len(preview) - 1} {
		prefix := bytes.Repeat([]byte{0x7a}, embeddedJPEGScanBufferSize-split)
		got, err := extractEmbeddedJPEG(bytes.NewReader(append(prefix, preview...)), 0)
		if err != nil {
			t.Fatalf("split %d: %v", split, err)
		}
		if !bytes.Equal(got, preview) {
			t.Fatalf("split %d: expected the complete embedded preview", split)
		}
	}
}

func TestScaleLargeReductionKeepsColorAndSize(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 3000, 2000), image.YCbCrSubsampleRatio420)
	y, cb, cr := color.RGBToYCbCr(200, 40, 90)
	for i := range src.Y {
		src.Y[i] = y
	}
	for i := range src.Cb {
		src.Cb[i] = cb
		src.Cr[i] = cr
	}

	scaled := Scale(src, 220)
	if scaled.Bounds().Dx() != 220 || scaled.Bounds().Dy() != 146 {
		t.Fatalf("expected 220x146, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}
	r, g, b, _ := scaled.At(110, 70).RGBA()
	if diff(r>>8, 200) > 3 || diff(g>>8, 40) > 3 || diff(b>>8, 90) > 3 {
		t.Fatalf("expected color near (200,40,90), got (%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func diff(a uint32, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
