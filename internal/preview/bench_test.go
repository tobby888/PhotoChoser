package preview

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func benchJPEG(b *testing.B, width int, height int) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 3), B: uint8(x ^ y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkLoadScaledPortraitJPEG(b *testing.B) {
	path := filepath.Join(b.TempDir(), "portrait.jpg")
	if err := os.WriteFile(path, addEXIFOrientation(benchJPEG(b, 6000, 4000), 6), 0o644); err != nil {
		b.Fatal(err)
	}
	for _, maxSide := range []int{220, 1616} {
		b.Run(strconv.Itoa(maxSide), func(b *testing.B) {
			for b.Loop() {
				if _, err := LoadScaled(path, maxSide); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLoadScaledRAWPreview(b *testing.B) {
	path := filepath.Join(b.TempDir(), "sample.arw")
	data := tiffWithPreviews(6, benchJPEG(b, 1616, 1080), benchJPEG(b, 160, 120), bytes.Repeat([]byte{0x5a}, 40<<20))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		b.Fatal(err)
	}
	benchLoadScaled(b, path)
}

func BenchmarkLoadScaledRAWPreviewScan(b *testing.B) {
	path := filepath.Join(b.TempDir(), "sample.cr3")
	data := bytes.Join([][]byte{
		tiffHeaderWithOrientation(6),
		benchJPEG(b, 160, 120),
		benchJPEG(b, 1616, 1080),
		bytes.Repeat([]byte{0x5a}, 40<<20),
	}, nil)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		b.Fatal(err)
	}
	benchLoadScaled(b, path)
}

func benchLoadScaled(b *testing.B, path string) {
	for _, maxSide := range []int{220, 1616} {
		b.Run(strconv.Itoa(maxSide), func(b *testing.B) {
			for b.Loop() {
				if _, err := LoadScaled(path, maxSide); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
