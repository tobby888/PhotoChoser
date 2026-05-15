package preview

import (
	"image"
	"os"
	"os/exec"
)

func loadNativePreview(path string) (image.Image, error) {
	tmp, err := os.CreateTemp("", "photochoser-preview-*.jpg")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := exec.Command("sips", "-s", "format", "jpeg", path, "--out", tmpPath).Run(); err != nil {
		return nil, err
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}
