package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"photochoser/internal/nativepicker"
	"photochoser/internal/photos"
	"photochoser/internal/preview"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type photoApp struct {
	window fyne.Window

	sourceDir     string
	sourceFiles   []string
	sourceBaseDir string
	targetDir     string
	recursive     bool
	transferMode  photos.TransferMode
	thumbCacheDir string

	items   []photos.Photo
	current int

	thumbs  sync.Map
	errors  sync.Map
	loading sync.Map

	sourceLabel *widget.Label
	targetLabel *widget.Label
	statusLabel *widget.Label
	countLabel  *widget.Label
	list        *widget.List
	mainImage   *canvas.Image
	titleLabel  *widget.Label
	moveButton  *widget.Button

	previewToken atomic.Int64
	scanToken    atomic.Int64
}

type thumbRow struct {
	widget.BaseWidget

	image    *canvas.Image
	name     *widget.Label
	selected *widget.Label
	error    *widget.Label
}

func newThumbRow() *thumbRow {
	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(92, 70))

	row := &thumbRow{
		image:    img,
		name:     widget.NewLabel(""),
		selected: widget.NewLabel(""),
		error:    widget.NewLabel(""),
	}
	row.name.Truncation = fyne.TextTruncateEllipsis
	row.selected.Alignment = fyne.TextAlignCenter
	row.error.TextStyle = fyne.TextStyle{Italic: true}
	row.ExtendBaseWidget(row)
	return row
}

func (row *thumbRow) CreateRenderer() fyne.WidgetRenderer {
	content := container.NewBorder(
		nil,
		nil,
		row.image,
		row.selected,
		container.NewVBox(row.name, row.error, widget.NewSeparator()),
	)
	return widget.NewSimpleRenderer(content)
}

func main() {
	thumbCacheDir, _ := os.MkdirTemp("", "photochoser-thumbs-*")
	if thumbCacheDir != "" {
		defer os.RemoveAll(thumbCacheDir)
	}

	fyneApp := app.NewWithID("com.photochoser.desktop")
	fyneApp.Settings().SetTheme(theme.LightTheme())

	w := fyneApp.NewWindow("PhotoChoser")
	w.Resize(fyne.NewSize(1180, 760))

	ui := &photoApp{
		window:        w,
		recursive:     true,
		current:       -1,
		transferMode:  photos.TransferMove,
		thumbCacheDir: thumbCacheDir,
	}
	ui.build()
	ui.bindKeys()

	w.ShowAndRun()
}

func (ui *photoApp) build() {
	ui.sourceLabel = widget.NewLabel("未导入照片")
	ui.targetLabel = widget.NewLabel("未选择目标目录")
	ui.statusLabel = widget.NewLabel("请选择照片文件夹导入，或扫描照片目录")
	ui.countLabel = widget.NewLabel("0 张 / 已选 0 张")
	ui.titleLabel = widget.NewLabel("没有照片")
	ui.titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	ui.mainImage = canvas.NewImageFromImage(nil)
	ui.mainImage.FillMode = canvas.ImageFillContain
	ui.mainImage.SetMinSize(fyne.NewSize(760, 560))

	recursiveCheck := widget.NewCheck("包含子目录", func(checked bool) {
		ui.recursive = checked
		if ui.sourceDir != "" {
			ui.scan()
		}
	})
	recursiveCheck.SetChecked(true)

	importButton := widget.NewButtonWithIcon("导入文件夹", theme.FolderOpenIcon(), func() {
		ui.openFolder("选择照片文件夹", ui.initialSourceDir(), func(path string) {
			ui.loadSourceFolder(path)
		})
	})
	sourceButton := widget.NewButtonWithIcon("扫描目录", theme.FolderOpenIcon(), func() {
		ui.openFolder("选择照片目录", ui.initialSourceDir(), func(path string) {
			ui.loadSourceFolder(path)
		})
	})
	targetButton := widget.NewButtonWithIcon("目标目录", theme.FolderIcon(), func() {
		ui.openFolder("选择目标目录", ui.targetDir, func(path string) {
			ui.targetDir = path
			ui.targetLabel.SetText(compactPath(path))
			ui.refreshStatus()
		})
	})
	actionChoice := widget.NewRadioGroup([]string{"移动", "复制"}, func(choice string) {
		if choice == "复制" {
			ui.transferMode = photos.TransferCopy
		} else {
			ui.transferMode = photos.TransferMove
		}
		ui.refreshTransferAction()
	})
	actionChoice.Horizontal = true
	actionChoice.SetSelected("移动")

	ui.moveButton = widget.NewButtonWithIcon("移动已选", theme.UploadIcon(), ui.transferSelected)
	ui.moveButton.Disable()

	ui.list = widget.NewList(
		func() int {
			return len(ui.items)
		},
		func() fyne.CanvasObject {
			return newThumbRow()
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row := object.(*thumbRow)
			item := ui.items[id]
			row.name.SetText(item.Name)
			row.selected.SetText("")
			if item.Selected {
				row.selected.SetText("✓")
			}
			row.error.SetText("")
			if errValue, ok := ui.errors.Load(item.Path); ok {
				row.error.SetText(errValue.(string))
			}
			if imgValue, ok := ui.thumbs.Load(item.Path); ok {
				row.image.Image = imgValue.(image.Image)
			} else {
				row.image.Image = nil
				ui.loadThumb(id, ui.scanToken.Load())
			}
			row.image.Refresh()
		},
	)
	ui.list.OnSelected = func(id widget.ListItemID) {
		ui.setCurrent(id)
	}

	top := container.NewVBox(
		container.NewHBox(importButton, sourceButton, ui.sourceLabel, recursiveCheck),
		container.NewHBox(targetButton, ui.targetLabel, actionChoice, ui.moveButton),
	)
	sidebar := container.NewBorder(ui.countLabel, nil, nil, nil, ui.list)
	previewPane := container.NewBorder(
		container.NewVBox(ui.titleLabel, ui.statusLabel),
		nil,
		nil,
		nil,
		container.NewPadded(ui.mainImage),
	)
	content := container.NewBorder(top, nil, sidebar, nil, previewPane)
	ui.window.SetContent(content)
}

func (ui *photoApp) openFolder(title string, startDir string, onPick func(string)) {
	path, err := nativepicker.PickFolder(title, startDir)
	if err == nativepicker.ErrCancelled {
		return
	}
	if err != nil {
		ui.statusLabel.SetText(err.Error())
		return
	}
	onPick(path)
}

func (ui *photoApp) openFiles(title string, startDir string, onPick func([]string)) {
	paths, err := nativepicker.PickFiles(title, startDir)
	if err == nativepicker.ErrCancelled {
		return
	}
	if err != nil {
		ui.statusLabel.SetText(err.Error())
		return
	}
	onPick(paths)
}

func (ui *photoApp) bindKeys() {
	canvas, ok := ui.window.Canvas().(desktop.Canvas)
	if !ok {
		return
	}
	canvas.SetOnKeyDown(func(event *fyne.KeyEvent) {
		switch event.Name {
		case fyne.KeyRight:
			ui.goTo(ui.current + 1)
		case fyne.KeyLeft:
			ui.goTo(ui.current - 1)
		case fyne.KeySpace:
			ui.toggleCurrent()
		case fyne.KeyReturn, fyne.KeyEnter:
			ui.transferSelected()
		case fyne.KeyM:
			ui.transferSelected()
		}
	})
}

func (ui *photoApp) scan() {
	items, err := photos.Scan(ui.sourceDir, ui.recursive)
	if err != nil {
		ui.statusLabel.SetText(err.Error())
		return
	}
	ui.loadItems(items)
}

func (ui *photoApp) loadSourceFolder(path string) {
	ui.sourceDir = path
	ui.sourceFiles = nil
	ui.sourceBaseDir = path
	ui.sourceLabel.SetText(compactPath(path))
	ui.scan()
}

func (ui *photoApp) importFiles(paths []string) {
	items := photos.FromPaths(paths)
	ui.sourceFiles = paths
	ui.sourceDir = ""
	ui.sourceBaseDir = commonDir(items)
	if len(items) == 0 {
		ui.statusLabel.SetText("没有导入受支持的照片文件")
		return
	}
	ui.sourceLabel.SetText(fmt.Sprintf("已导入 %d 张", len(items)))
	ui.loadItems(items)
}

func (ui *photoApp) loadItems(items []photos.Photo) {
	ui.items = items
	ui.current = -1
	ui.thumbs = sync.Map{}
	ui.errors = sync.Map{}
	ui.loading = sync.Map{}
	ui.scanToken.Add(1)
	ui.list.Refresh()
	if len(items) > 0 {
		ui.setCurrent(0)
		ui.list.Select(0)
		ui.preloadThumbs(ui.scanToken.Load())
	} else {
		ui.mainImage.Image = nil
		ui.mainImage.Refresh()
		ui.titleLabel.SetText("没有照片")
	}
	ui.refreshStatus()
}

func (ui *photoApp) setCurrent(id int) {
	if id < 0 || id >= len(ui.items) {
		return
	}
	ui.current = id
	item := ui.items[id]
	ui.titleLabel.SetText(selectionMark(item.Selected) + item.Name)
	ui.statusLabel.SetText("正在加载预览...")

	token := ui.previewToken.Add(1)
	go func(path string, name string, selected bool) {
		img, err := preview.LoadScaled(path, 1600)
		fyne.Do(func() {
			if ui.previewToken.Load() != token {
				return
			}
			if err != nil {
				ui.mainImage.Image = nil
				ui.mainImage.Refresh()
				ui.statusLabel.SetText("无法预览：" + err.Error())
				return
			}
			ui.mainImage.Image = img
			ui.mainImage.Refresh()
			ui.titleLabel.SetText(selectionMark(selected) + name)
			ui.refreshStatus()
		})
	}(item.Path, item.Name, item.Selected)
}

func (ui *photoApp) loadThumb(id int, token int64) {
	go ui.loadThumbNow(id, token)
}

func (ui *photoApp) loadThumbNow(id int, token int64) {
	if ui.scanToken.Load() != token {
		return
	}
	if id < 0 || id >= len(ui.items) {
		return
	}
	item := ui.items[id]
	if ui.scanToken.Load() != token {
		return
	}
	if _, ok := ui.thumbs.Load(item.Path); ok {
		return
	}
	if _, loaded := ui.loading.LoadOrStore(item.Path, struct{}{}); loaded {
		return
	}
	img, err := ui.loadThumbImage(item.Path)
	fyne.Do(func() {
		ui.loading.Delete(item.Path)
		if ui.scanToken.Load() != token {
			return
		}
		if err != nil {
			ui.errors.Store(item.Path, err.Error())
		} else {
			ui.thumbs.Store(item.Path, img)
		}
		if id < len(ui.items) {
			ui.list.RefreshItem(id)
		}
	})
}

func (ui *photoApp) preloadThumbs(token int64) {
	total := len(ui.items)
	if total == 0 {
		return
	}
	workers := min(total, max(2, runtime.NumCPU()))
	workers = min(workers, 6)
	jobs := make(chan int, total)

	for range workers {
		go func() {
			for id := range jobs {
				if ui.scanToken.Load() != token {
					return
				}
				ui.loadThumbNow(id, token)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for id := range ui.items {
			if ui.scanToken.Load() != token {
				return
			}
			jobs <- id
		}
	}()
}

func (ui *photoApp) loadThumbImage(path string) (image.Image, error) {
	return preview.LoadCachedScaled(path, 220, ui.thumbCacheDir)
}

func (ui *photoApp) goTo(id int) {
	if len(ui.items) == 0 {
		return
	}
	if id < 0 {
		id = 0
	}
	if id >= len(ui.items) {
		id = len(ui.items) - 1
	}
	ui.list.Select(id)
	ui.setCurrent(id)
}

func (ui *photoApp) toggleCurrent() {
	if ui.current < 0 || ui.current >= len(ui.items) {
		return
	}
	ui.items[ui.current].Selected = !ui.items[ui.current].Selected
	ui.list.RefreshItem(ui.current)
	ui.titleLabel.SetText(selectionMark(ui.items[ui.current].Selected) + ui.items[ui.current].Name)
	ui.refreshStatus()
	ui.goTo(ui.current + 1)
}

func (ui *photoApp) transferSelected() {
	if selectedCount(ui.items) == 0 {
		ui.statusLabel.SetText("还没有挑选照片")
		return
	}
	if ui.targetDir == "" {
		ui.statusLabel.SetText("请先选择目标目录")
		return
	}

	result, errs := photos.TransferSelected(ui.items, ui.targetDir, ui.transferMode)
	action := ui.transferActionText()
	if len(errs) > 0 {
		ui.statusLabel.SetText(fmt.Sprintf("已%s %d 张，%d 个文件失败：%s", action, result.Count, len(errs), errs[0]))
	} else {
		ui.statusLabel.SetText(fmt.Sprintf("已%s %d 张照片到 %s", action, result.Count, compactPath(ui.targetDir)))
	}
	message := ui.statusLabel.Text
	if ui.transferMode == photos.TransferMove {
		ui.removeTransferredFromList(result.Paths)
	} else {
		ui.clearTransferredSelection(result.Paths)
	}
	ui.statusLabel.SetText(message)
}

func (ui *photoApp) removeTransferredFromList(paths []string) {
	transferred := pathSet(paths)
	remaining := make([]photos.Photo, 0, len(ui.items))
	for _, item := range ui.items {
		if _, ok := transferred[item.Path]; !ok {
			remaining = append(remaining, item)
		}
	}
	ui.loadItems(remaining)
}

func (ui *photoApp) clearTransferredSelection(paths []string) {
	transferred := pathSet(paths)
	for i := range ui.items {
		if _, ok := transferred[ui.items[i].Path]; ok {
			ui.items[i].Selected = false
			ui.list.RefreshItem(i)
		}
	}
	ui.refreshStatus()
}

func (ui *photoApp) refreshStatus() {
	total := len(ui.items)
	selected := selectedCount(ui.items)
	ui.countLabel.SetText(fmt.Sprintf("%d 张 / 已选 %d 张", total, selected))
	if selected > 0 && ui.targetDir != "" {
		ui.moveButton.Enable()
	} else {
		ui.moveButton.Disable()
	}
	if total > 0 && ui.current >= 0 {
		ui.statusLabel.SetText(fmt.Sprintf("%d / %d", ui.current+1, total))
	}
	ui.refreshTransferAction()
}

func (ui *photoApp) refreshTransferAction() {
	if ui.moveButton == nil {
		return
	}
	if ui.transferMode == photos.TransferCopy {
		ui.moveButton.SetText("复制已选")
		ui.moveButton.SetIcon(theme.ContentCopyIcon())
		return
	}
	ui.moveButton.SetText("移动已选")
	ui.moveButton.SetIcon(theme.UploadIcon())
}

func (ui *photoApp) transferActionText() string {
	if ui.transferMode == photos.TransferCopy {
		return "复制"
	}
	return "移动"
}

func pathSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		set[path] = struct{}{}
	}
	return set
}

func selectedCount(items []photos.Photo) int {
	count := 0
	for _, item := range items {
		if item.Selected {
			count++
		}
	}
	return count
}

func selectionMark(selected bool) string {
	if selected {
		return "✓ "
	}
	return ""
}

func compactPath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) <= 3 {
		return clean
	}
	if filepath.IsAbs(clean) && parts[0] == "" {
		return string(filepath.Separator) + filepath.Join(parts[1], "...", parts[len(parts)-2], parts[len(parts)-1])
	}
	return filepath.Join(parts[0], "...", parts[len(parts)-2], parts[len(parts)-1])
}

func (ui *photoApp) initialSourceDir() string {
	if ui.sourceBaseDir != "" {
		return ui.sourceBaseDir
	}
	if len(ui.sourceFiles) > 0 {
		return filepath.Dir(ui.sourceFiles[0])
	}
	return ""
}

func commonDir(items []photos.Photo) string {
	if len(items) == 0 {
		return ""
	}
	dir := filepath.Dir(items[0].Path)
	for _, item := range items[1:] {
		next := filepath.Dir(item.Path)
		if next != dir {
			return ""
		}
	}
	return dir
}
