package preview

import (
	"image"
	"os"
	"os/exec"
)

func loadNativePreview(path string) (image.Image, error) {
	tmp, err := os.CreateTemp("", "photochoser-preview-*.png")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	script := `
param([string]$inputPath, [string]$outputPath)
Add-Type -AssemblyName PresentationCore
$stream = [System.IO.File]::OpenRead($inputPath)
try {
	$decoder = [System.Windows.Media.Imaging.BitmapDecoder]::Create($stream, [System.Windows.Media.Imaging.BitmapCreateOptions]::PreservePixelFormat, [System.Windows.Media.Imaging.BitmapCacheOption]::OnLoad)
	$frame = $decoder.Frames[0]
	$encoder = New-Object System.Windows.Media.Imaging.PngBitmapEncoder
	$encoder.Frames.Add([System.Windows.Media.Imaging.BitmapFrame]::Create($frame))
	$outStream = [System.IO.File]::Create($outputPath)
	try {
		$encoder.Save($outStream)
	} finally {
		$outStream.Close()
	}
} finally {
	$stream.Close()
}`
	if err := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script, path, tmpPath).Run(); err != nil {
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
