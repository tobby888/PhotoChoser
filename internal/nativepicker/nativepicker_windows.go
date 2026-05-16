package nativepicker

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	maxPath = 260

	ofnAllowMultiselect = 0x00000200
	ofnExplorer         = 0x00080000
	ofnFileMustExist    = 0x00001000
	ofnHideReadOnly     = 0x00000004
	ofnPathMustExist    = 0x00000800

	bifReturnOnlyFSDirs = 0x00000001
	bifEditBox          = 0x00000010
	bifNewDialogStyle   = 0x00000040
	bifShareable        = 0x00008000

	bffmInitialized   = 1
	bffmSetSelectionW = 0x0467

	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
)

var (
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	procCoUninitialize       = ole32.NewProc("CoUninitialize")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procSendMessageW         = user32.NewProc("SendMessageW")
)

type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

type browseInfoW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

func PickFiles(title string, startDir string) ([]string, error) {
	initialized, err := initializeCOM()
	if err != nil {
		return nil, err
	}
	if initialized {
		defer procCoUninitialize.Call()
	}

	buffer := make([]uint16, 65536)
	ofn := openFileNameW{
		lStructSize:     uint32(unsafe.Sizeof(openFileNameW{})),
		lpstrFilter:     utf16Filter("Photos", "*.jpg;*.jpeg;*.png;*.tif;*.tiff;*.arw;*.srf;*.sr2;*.nef;*.nrw;*.cr2;*.cr3;*.crw;*.raf;*.rw2;*.orf;*.dng;*.pef;*.3fr;*.fff;*.iiq;*.mos;*.mrw;*.x3f;*.kdc;*.erf;*.mef;*.rwl", "All files", "*.*"),
		nFilterIndex:    1,
		lpstrFile:       &buffer[0],
		nMaxFile:        uint32(len(buffer)),
		lpstrInitialDir: utf16PtrIfValidDir(startDir),
		lpstrTitle:      syscall.StringToUTF16Ptr(title),
		flags:           ofnExplorer | ofnAllowMultiselect | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly,
	}

	ret, _, callErr := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		if dialogErr := commDlgError(); dialogErr != nil {
			return nil, dialogErr
		}
		if callErr != syscall.Errno(0) {
			return nil, callErr
		}
		return nil, ErrCancelled
	}

	paths := parseOpenFileNameBuffer(buffer)
	if len(paths) == 0 {
		return nil, ErrCancelled
	}
	return paths, nil
}

func PickFolder(title string, startDir string) (string, error) {
	initialized, err := initializeCOM()
	if err != nil {
		return "", err
	}
	if initialized {
		defer procCoUninitialize.Call()
	}

	displayName := make([]uint16, maxPath)
	var startPtr *uint16
	if isValidDir(startDir) {
		startPtr = syscall.StringToUTF16Ptr(startDir)
	}

	callback := syscall.NewCallback(func(hwnd uintptr, msg uint32, lParam uintptr, lpData uintptr) uintptr {
		if msg == bffmInitialized && lpData != 0 {
			procSendMessageW.Call(hwnd, bffmSetSelectionW, 1, lpData)
		}
		return 0
	})

	bi := browseInfoW{
		pszDisplayName: &displayName[0],
		lpszTitle:      syscall.StringToUTF16Ptr(title),
		ulFlags:        bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle | bifShareable,
		lpfn:           callback,
	}
	if startPtr != nil {
		bi.lParam = uintptr(unsafe.Pointer(startPtr))
	}

	pidl, _, callErr := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", ErrCancelled
	}
	defer procCoTaskMemFree.Call(pidl)

	pathBuffer := make([]uint16, maxPath)
	ok, _, callErr := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuffer[0])))
	if ok == 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", errors.New("selected folder path could not be resolved")
	}

	path := syscall.UTF16ToString(pathBuffer)
	if strings.TrimSpace(path) == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func initializeCOM() (bool, error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if hr == 0 || hr == 1 {
		return true, nil
	}
	if uint32(hr) == rpcEChangedMode {
		return false, nil
	}
	return false, syscall.Errno(hr)
}

func utf16Filter(parts ...string) *uint16 {
	values := make([]uint16, 0, 256)
	for _, part := range parts {
		values = append(values, syscall.StringToUTF16(part)...)
	}
	values = append(values, 0)
	return &values[0]
}

func utf16PtrIfValidDir(path string) *uint16 {
	if !isValidDir(path) {
		return nil
	}
	return syscall.StringToUTF16Ptr(path)
}

func isValidDir(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	info, err := syscall.GetFileAttributes(syscall.StringToUTF16Ptr(abs))
	return err == nil && info&syscall.FILE_ATTRIBUTE_DIRECTORY != 0
}

func parseOpenFileNameBuffer(buffer []uint16) []string {
	parts := splitDoubleNullUTF16(buffer)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts
	}

	dir := parts[0]
	paths := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		if name != "" {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths
}

func splitDoubleNullUTF16(buffer []uint16) []string {
	parts := make([]string, 0, 8)
	start := 0
	for i, r := range buffer {
		if r != 0 {
			continue
		}
		if i == start {
			break
		}
		parts = append(parts, syscall.UTF16ToString(buffer[start:i]))
		start = i + 1
	}
	return parts
}

func commDlgError() error {
	code, _, _ := procCommDlgExtendedError.Call()
	if code == 0 {
		return nil
	}
	return syscall.Errno(code)
}
