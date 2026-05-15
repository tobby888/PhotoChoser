//go:build !darwin && !windows

package preview

import (
	"errors"
	"image"
)

func loadNativePreview(path string) (image.Image, error) {
	return nil, errors.New("native RAW preview fallback is not implemented on this platform")
}
