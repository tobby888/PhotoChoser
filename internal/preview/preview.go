package preview

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

const (
	embeddedJPEGScanBufferSize = 256 * 1024
	embeddedJPEGMaxBytes       = 128 << 20
)

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8?a", gif.Decode, gif.DecodeConfig)
}

func LoadScaled(path string, maxSide int) (image.Image, error) {
	if maxSide > 0 && !isStandardImage(path) {
		if img, err := loadEmbeddedJPEGScaled(path, maxSide); err == nil {
			return Scale(img, maxSide), nil
		}
	}

	img, err := Load(path)
	if err != nil {
		return nil, err
	}
	if maxSide <= 0 {
		return img, nil
	}
	return Scale(img, maxSide), nil
}

func loadEmbeddedJPEGScaled(path string, maxSide int) (image.Image, error) {
	orientation := ReadOrientation(path)
	jpegBytes, err := extractEmbeddedJPEGForMaxSide(path, maxSide)
	if err != nil {
		return nil, err
	}
	if orientation == orientationNormal {
		orientation = readOrientationFromBytes(jpegBytes)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, err
	}
	return ApplyOrientation(img, orientation), nil
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

	img, fallbackErr := loadRAWFallback(path)
	if fallbackErr == nil {
		return img, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fallbackErr
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
	return extractEmbeddedJPEGForMaxSide(path, 0)
}

func extractEmbeddedJPEGForMaxSide(path string, maxSide int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return extractEmbeddedJPEG(file, maxSide)
}

func extractEmbeddedJPEG(reader io.Reader, maxSide int) ([]byte, error) {
	var choice embeddedJPEGChoice

	buffered := bufio.NewReaderSize(reader, embeddedJPEGScanBufferSize)
	state := 0
	var candidate []byte
	for {
		b, err := buffered.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if candidate != nil {
			candidate = append(candidate, b)
			if len(candidate) > embeddedJPEGMaxBytes {
				candidate = nil
				state = 0
				continue
			}
			if len(candidate) >= 2 && candidate[len(candidate)-2] == 0xff && candidate[len(candidate)-1] == 0xd9 {
				choice.keep(candidate, maxSide)
				candidate = nil
				state = 0
			}
			continue
		}

		switch state {
		case 0:
			if b == 0xff {
				state = 1
			}
		case 1:
			switch b {
			case 0xd8:
				state = 2
			case 0xff:
				state = 1
			default:
				state = 0
			}
		case 2:
			if b == 0xff {
				candidate = []byte{0xff, 0xd8, 0xff}
			} else {
				state = 0
			}
		}
	}

	if best := choice.best(); len(best) > 0 {
		return best, nil
	}
	return nil, errors.New("no embedded JPEG preview found")
}

type embeddedJPEGChoice struct {
	enough       []byte
	enoughArea   int
	fallback     []byte
	fallbackArea int
}

func (choice *embeddedJPEGChoice) keep(candidate []byte, maxSide int) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(candidate))
	if err != nil {
		return
	}

	area := cfg.Width * cfg.Height
	longSide := max(cfg.Width, cfg.Height)
	if maxSide > 0 && longSide >= maxSide {
		if len(choice.enough) == 0 || area < choice.enoughArea || (area == choice.enoughArea && len(candidate) < len(choice.enough)) {
			choice.enough = candidate
			choice.enoughArea = area
		}
		return
	}

	if area > choice.fallbackArea || (area == choice.fallbackArea && len(candidate) > len(choice.fallback)) {
		choice.fallback = candidate
		choice.fallbackArea = area
	}
}

func (choice *embeddedJPEGChoice) best() []byte {
	if len(choice.enough) > 0 {
		return choice.enough
	}
	return choice.fallback
}

func isStandardImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".tif", ".tiff":
		return true
	default:
		return false
	}
}
