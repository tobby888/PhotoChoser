package photos

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Photo struct {
	Path     string
	Name     string
	Ext      string
	Selected bool
}

type TransferMode string

const (
	TransferMove TransferMode = "move"
	TransferCopy TransferMode = "copy"
)

type TransferResult struct {
	Count int
	Paths []string
}

var supportedExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".tif": {}, ".tiff": {},
	".arw": {}, ".srf": {}, ".sr2": {},
	".nef": {}, ".nrw": {},
	".cr2": {}, ".cr3": {}, ".crw": {},
	".raf": {},
	".rw2": {},
	".orf": {},
	".dng": {},
	".pef": {},
	".3fr": {}, ".fff": {}, ".iiq": {},
	".mos": {}, ".mrw": {}, ".x3f": {}, ".kdc": {},
	".erf": {}, ".mef": {}, ".rwl": {},
}

func IsSupported(path string) bool {
	if isAppleDouble(path) {
		return false
	}
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func IsRAW(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".tif", ".tiff":
		return false
	default:
		return IsSupported(path)
	}
}

func SupportedExtensions() []string {
	extensions := make([]string, 0, len(supportedExtensions))
	for ext := range supportedExtensions {
		extensions = append(extensions, ext)
	}
	sort.Strings(extensions)
	return extensions
}

func Scan(root string, recursive bool) ([]Photo, error) {
	if root == "" {
		return nil, errors.New("photo directory is empty")
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	var items []Photo
	if recursive {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if IsSupported(path) {
				items = append(items, photoFromPath(path))
			}
			return nil
		})
	} else {
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return nil, readErr
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if IsSupported(path) {
				items = append(items, photoFromPath(path))
			}
		}
	}
	if err != nil {
		return nil, err
	}

	sortPhotos(items)
	return items, nil
}

func FromPaths(paths []string) []Photo {
	seen := make(map[string]struct{}, len(paths))
	items := make([]Photo, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}

		info, err := os.Stat(clean)
		if err != nil || info.IsDir() || !IsSupported(clean) {
			continue
		}
		items = append(items, photoFromPath(clean))
	}
	sortPhotos(items)
	return items
}

func MoveSelected(items []Photo, targetDir string) (int, []error) {
	result, errs := TransferSelected(items, targetDir, TransferMove)
	return result.Count, errs
}

func CopySelected(items []Photo, targetDir string) (int, []error) {
	result, errs := TransferSelected(items, targetDir, TransferCopy)
	return result.Count, errs
}

func TransferSelected(items []Photo, targetDir string, mode TransferMode) (TransferResult, []error) {
	if targetDir == "" {
		return TransferResult{}, []error{errors.New("target directory is empty")}
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return TransferResult{}, []error{err}
	}

	var result TransferResult
	var errs []error
	for _, item := range items {
		if !item.Selected {
			continue
		}
		if err := transferOne(item.Path, targetDir, mode); err != nil {
			errs = append(errs, err)
			continue
		}
		transferSidecarXMP(item.Path, targetDir, mode)
		result.Count++
		result.Paths = append(result.Paths, item.Path)
	}
	return result, errs
}

func photoFromPath(path string) Photo {
	return Photo{
		Path: filepath.Clean(path),
		Name: filepath.Base(path),
		Ext:  strings.ToLower(filepath.Ext(path)),
	}
}

func naturalishKey(name string) string {
	return strings.ToLower(name)
}

func isAppleDouble(path string) bool {
	return strings.HasPrefix(filepath.Base(path), "._")
}

func sortPhotos(items []Photo) {
	sort.Slice(items, func(i, j int) bool {
		return naturalishKey(items[i].Name) < naturalishKey(items[j].Name)
	})
}

func moveOne(src string, targetDir string) error {
	dst := uniqueDestination(filepath.Join(targetDir, filepath.Base(src)))
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyThenRemove(src, dst)
}

func copyOne(src string, targetDir string) error {
	dst := uniqueDestination(filepath.Join(targetDir, filepath.Base(src)))
	return copyFile(src, dst)
}

func transferOne(src string, targetDir string, mode TransferMode) error {
	switch mode {
	case TransferMove:
		return moveOne(src, targetDir)
	case TransferCopy:
		return copyOne(src, targetDir)
	default:
		return fmt.Errorf("unsupported transfer mode: %s", mode)
	}
}

func transferSidecarXMP(photoPath string, targetDir string, mode TransferMode) {
	sidecar := strings.TrimSuffix(photoPath, filepath.Ext(photoPath)) + ".xmp"
	if _, err := os.Stat(sidecar); err == nil {
		_ = transferOne(sidecar, targetDir, mode)
		return
	}
	sidecar = strings.TrimSuffix(photoPath, filepath.Ext(photoPath)) + ".XMP"
	if _, err := os.Stat(sidecar); err == nil {
		_ = transferOne(sidecar, targetDir, mode)
	}
}

func uniqueDestination(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%03d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func copyThenRemove(src string, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}

	info, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, info.Mode())
	}
	return nil
}
