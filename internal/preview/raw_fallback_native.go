//go:build !libraw

package preview

import "image"

func loadRAWFallback(path string) (image.Image, error) {
	return loadNativePreview(path)
}
