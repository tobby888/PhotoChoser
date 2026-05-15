package preview

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8?a", gif.Decode, gif.DecodeConfig)
}

func LoadScaled(path string, maxSide int) (image.Image, error) {
	img, err := Load(path)
	if err != nil {
		return nil, err
	}
	if maxSide <= 0 {
		return img, nil
	}
	return Scale(img, maxSide), nil
}

func Load(path string) (image.Image, error) {
	orientation := ReadOrientation(path)
	if isStandardImage(path) {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err == nil {
			return ApplyOrientation(img, orientation), nil
		}
	}

	jpegBytes, err := ExtractEmbeddedJPEG(path)
	if err == nil {
		if orientation == orientationNormal {
			orientation = readOrientationFromBytes(jpegBytes)
		}
		img, err := jpeg.Decode(bytes.NewReader(jpegBytes))
		if err == nil {
			return ApplyOrientation(img, orientation), nil
		}
	}

	img, nativeErr := loadNativePreview(path)
	if nativeErr == nil {
		return img, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, nativeErr
}

func Scale(src image.Image, maxSide int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxSide && height <= maxSide {
		return src
	}

	scale := float64(maxSide) / float64(width)
	if height > width {
		scale = float64(maxSide) / float64(height)
	}
	dstWidth := max(1, int(float64(width)*scale))
	dstHeight := max(1, int(float64(height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func ExtractEmbeddedJPEG(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var best []byte
	var bestArea int
	for _, candidate := range jpegCandidates(data) {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(candidate))
		if err != nil {
			continue
		}
		area := cfg.Width * cfg.Height
		if area > bestArea || (area == bestArea && len(candidate) > len(best)) {
			best = candidate
			bestArea = area
		}
	}
	if len(best) == 0 {
		return nil, errors.New("no embedded JPEG preview found")
	}
	return best, nil
}

func jpegCandidates(data []byte) [][]byte {
	var candidates [][]byte
	for i := 0; i < len(data)-3; i++ {
		if data[i] != 0xff || data[i+1] != 0xd8 || data[i+2] != 0xff {
			continue
		}
		endOffset := bytes.Index(data[i+2:], []byte{0xff, 0xd9})
		if endOffset < 0 {
			continue
		}
		end := i + 2 + endOffset + 2
		if end > i {
			candidates = append(candidates, data[i:end])
			i = end - 1
		}
	}
	return candidates
}

func isStandardImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".tif", ".tiff":
		return true
	default:
		return false
	}
}
