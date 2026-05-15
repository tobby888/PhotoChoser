package nativepicker

import (
	"errors"
	"os/exec"
	"strings"
)

func PickFiles(title string, startDir string) ([]string, error) {
	script := `
param([string]$title, [string]$startDir)
Add-Type -AssemblyName System.Windows.Forms
$dlg = New-Object System.Windows.Forms.OpenFileDialog
$dlg.Title = $title
$dlg.Multiselect = $true
$dlg.Filter = 'Photos|*.jpg;*.jpeg;*.png;*.tif;*.tiff;*.arw;*.srf;*.sr2;*.nef;*.nrw;*.cr2;*.cr3;*.crw;*.raf;*.rw2;*.orf;*.dng;*.pef;*.3fr;*.fff;*.iiq;*.mos;*.mrw;*.x3f;*.kdc;*.erf;*.mef;*.rwl|All files|*.*'
if ($startDir -and (Test-Path -LiteralPath $startDir)) {
	$dlg.InitialDirectory = $startDir
}
if ($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	$dlg.FileNames -join [Environment]::NewLine
	exit 0
}
exit 2`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script, title, startDir).Output()
	if err != nil {
		if isExitCode(err, 2) {
			return nil, ErrCancelled
		}
		return nil, err
	}
	return parsePickerLines(string(out))
}

func PickFolder(title string, startDir string) (string, error) {
	script := `
param([string]$title, [string]$startDir)
Add-Type -AssemblyName System.Windows.Forms
$dlg = New-Object System.Windows.Forms.FolderBrowserDialog
$dlg.Description = $title
$dlg.ShowNewFolderButton = $true
if ($startDir -and (Test-Path -LiteralPath $startDir)) {
	$dlg.SelectedPath = $startDir
}
if ($dlg.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	$dlg.SelectedPath
	exit 0
}
exit 2`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script, title, startDir).Output()
	if err != nil {
		if isExitCode(err, 2) {
			return "", ErrCancelled
		}
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func parsePickerLines(output string) ([]string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, ErrCancelled
	}
	lines := strings.Split(trimmed, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil, ErrCancelled
	}
	return paths, nil
}

func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}
