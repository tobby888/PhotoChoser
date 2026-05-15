//go:build !darwin && !windows

package nativepicker

import "fmt"

func PickFiles(title string, startDir string) ([]string, error) {
	return nil, fmt.Errorf("native multi-file picker is not implemented on this platform")
}

func PickFolder(title string, startDir string) (string, error) {
	return "", fmt.Errorf("native folder picker is not implemented on this platform")
}
