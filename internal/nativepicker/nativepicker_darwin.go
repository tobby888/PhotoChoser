package nativepicker

import (
	"os/exec"
	"strings"
)

const cancelledMarker = "__PHOTOCHOSER_CANCELLED__"

func PickFiles(title string, startDir string) ([]string, error) {
	script := `
on run argv
	set promptText to item 1 of argv
	set startDir to item 2 of argv
	try
		if startDir is "" then
			set chosenFiles to choose file with prompt promptText with multiple selections allowed
		else
			set chosenFiles to choose file with prompt promptText default location (POSIX file startDir) with multiple selections allowed
		end if
		set output to ""
		repeat with f in chosenFiles
			set output to output & POSIX path of f & linefeed
		end repeat
		return output
	on error number -128
		return "` + cancelledMarker + `"
	end try
end run`
	out, err := exec.Command("osascript", "-e", script, title, startDir).Output()
	if err != nil {
		return nil, err
	}
	return parsePickerLines(string(out))
}

func PickFolder(title string, startDir string) (string, error) {
	script := `
on run argv
	set promptText to item 1 of argv
	set startDir to item 2 of argv
	try
		if startDir is "" then
			set chosenFolder to choose folder with prompt promptText
		else
			set chosenFolder to choose folder with prompt promptText default location (POSIX file startDir)
		end if
		return POSIX path of chosenFolder
	on error number -128
		return "` + cancelledMarker + `"
	end try
end run`
	out, err := exec.Command("osascript", "-e", script, title, startDir).Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" || path == cancelledMarker {
		return "", ErrCancelled
	}
	return path, nil
}

func parsePickerLines(output string) ([]string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == cancelledMarker {
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
