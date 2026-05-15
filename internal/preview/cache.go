package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
)

func LoadCachedScaled(path string, maxSide int, cacheDir string) (image.Image, error) {
	if cacheDir == "" {
		return LoadScaled(path, maxSide)
	}

	cachedPath, err := cachePath(path, maxSide, cacheDir)
	if err != nil {
		return LoadScaled(path, maxSide)
	}
	if img, err := loadCachedJPEG(cachedPath); err == nil {
		return img, nil
	}

	img, err := LoadScaled(path, maxSide)
	if err != nil {
		return nil, err
	}
	_ = saveCachedJPEG(cachedPath, img)
	return img, nil
}

func cachePath(path string, maxSide int, cacheDir string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d", abs, maxSide, info.Size(), info.ModTime().UnixNano())))
	return filepath.Join(cacheDir, hex.EncodeToString(hash[:])+".jpg"), nil
}

func loadCachedJPEG(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return jpeg.Decode(file)
}

func saveCachedJPEG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "thumb-*.jpg")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	encodeErr := jpeg.Encode(tmp, img, &jpeg.Options{Quality: 82})
	closeErr := tmp.Close()
	if encodeErr != nil {
		_ = os.Remove(tmpPath)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	return os.Rename(tmpPath, path)
}
