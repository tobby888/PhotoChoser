package preview

import (
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

	_ "golang.org/x/image/tiff"
)

const (
	embeddedJPEGScanBufferSize = 256 * 1024
	embeddedJPEGMaxBytes       = 128 << 20
)

var jpegEOI = []byte{0xff, 0xd9}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8?a", gif.Decode, gif.DecodeConfig)
}

// LoadScaled returns an oriented *image.RGBA whose longest side is at most
// maxSide, ready for display without further conversion.
func LoadScaled(path string, maxSide int) (image.Image, error) {
	if maxSide <= 0 {
		return Load(path)
	}
	if !isStandardImage(path) {
		if img, orientation, err := loadEmbeddedJPEGScaled(path, maxSide); err == nil {
			return scaleOriented(img, orientation, maxSide), nil
		}
	}

	img, orientation, err := loadSource(path)
	if err != nil {
		return nil, err
	}
	return scaleOriented(img, orientation, maxSide), nil
}

// scaleOriented scales before rotating so orientation only touches the small
// output image.
func scaleOriented(img image.Image, orientation int, maxSide int) *image.RGBA {
	return ToRGBA(ApplyOrientation(Scale(img, maxSide), orientation))
}

func loadEmbeddedJPEGScaled(path string, maxSide int) (image.Image, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	if jpegBytes, orientation, err := readTIFFPreview(file, maxSide); err == nil {
		if img, orientation, err := decodeEmbeddedJPEG(jpegBytes, orientation); err == nil {
			return img, orientation, nil
		}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	jpegBytes, err := extractEmbeddedJPEG(file, maxSide)
	if err != nil {
		return nil, 0, err
	}
	return decodeEmbeddedJPEG(jpegBytes, ReadOrientation(path))
}

func decodeEmbeddedJPEG(jpegBytes []byte, orientation int) (image.Image, int, error) {
	if orientation == orientationNormal {
		orientation = readOrientationFromBytes(jpegBytes)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, 0, err
	}
	return img, orientation, nil
}

func Load(path string) (image.Image, error) {
	img, orientation, err := loadSource(path)
	if err != nil {
		return nil, err
	}
	return ApplyOrientation(img, orientation), nil
}

// loadSource decodes the full-size image and returns the orientation that
// still has to be applied to it.
func loadSource(path string) (image.Image, int, error) {
	orientation := ReadOrientation(path)
	if isStandardImage(path) {
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err == nil {
			return img, orientation, nil
		}
	}

	jpegBytes, err := ExtractEmbeddedJPEG(path)
	if err == nil {
		if orientation == orientationNormal {
			orientation = readOrientationFromBytes(jpegBytes)
		}
		img, err := jpeg.Decode(bytes.NewReader(jpegBytes))
		if err == nil {
			return img, orientation, nil
		}
	}

	img, fallbackErr := loadRAWFallback(path)
	if fallbackErr == nil {
		return img, orientationNormal, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return nil, 0, fallbackErr
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

// extractEmbeddedJPEG scans the whole stream for SOI...EOI byte runs. It is
// the slow fallback for RAW layouts whose previews readTIFFPreview cannot find.
func extractEmbeddedJPEG(reader io.Reader, maxSide int) ([]byte, error) {
	var choice embeddedJPEGChoice

	buf := make([]byte, embeddedJPEGScanBufferSize)
	state := 0
	var candidate []byte
	for {
		n, readErr := reader.Read(buf)
		chunk := buf[:n]
		for len(chunk) > 0 {
			if candidate != nil {
				end := -1
				if candidate[len(candidate)-1] == 0xff && chunk[0] == 0xd9 {
					end = 1
				} else if i := bytes.Index(chunk, jpegEOI); i >= 0 {
					end = i + 2
				}
				if end < 0 {
					candidate = append(candidate, chunk...)
					chunk = nil
				} else {
					candidate = append(candidate, chunk[:end]...)
					chunk = chunk[end:]
				}
				if len(candidate) > embeddedJPEGMaxBytes {
					candidate = nil
					state = 0
					continue
				}
				if end >= 0 {
					choice.keep(candidate, maxSide)
					candidate = nil
					state = 0
				}
				continue
			}

			switch state {
			case 0:
				i := bytes.IndexByte(chunk, 0xff)
				if i < 0 {
					chunk = nil
					continue
				}
				chunk = chunk[i+1:]
				state = 1
			case 1:
				switch chunk[0] {
				case 0xd8:
					state = 2
				case 0xff:
					state = 1
				default:
					state = 0
				}
				chunk = chunk[1:]
			case 2:
				if chunk[0] == 0xff {
					candidate = []byte{0xff, 0xd8, 0xff}
				} else {
					state = 0
				}
				chunk = chunk[1:]
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
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
