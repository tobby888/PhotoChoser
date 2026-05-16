package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

const (
	thumbMaxSide        = 220
	previewMaxSide      = 1280
	previewLoadWait     = 45 * time.Millisecond
	previewPrefetchNext = 3
	previewPrefetchWait = 140 * time.Millisecond
	previewMemoryLimit  = previewPrefetchNext + 3
	thumbMemoryLimit    = 360
	thumbWorkerLimit    = 3
	thumbQueueBuffer    = 2048
	thumbVisibleRadius  = 14
	thumbStaleDistance  = 30
	memoryTrimDelay     = 2 * time.Second
	autoScanInterval    = 6 * time.Second
)

type previewResult struct {
	img image.Image
	err error
}

type previewJob struct {
	done   chan struct{}
	result previewResult
}

type thumbJob struct {
	id              int
	token           int64
	backgroundToken int64
}

type photoApp struct {
	window fyne.Window

	sourceDir       string
	sourceFiles     []string
	sourceBaseDir   string
	targetDir       string
	recursive       bool
	transferMode    photos.TransferMode
	previewCacheDir string

	items   []photos.Photo
	current int

	thumbs         *imageCache
	previewImages  *imageCache
	errors         sync.Map
	loading        sync.Map
	previewLoading sync.Map

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

	thumbPreloadStarted  atomic.Bool
	memoryTrimPending    atomic.Bool
	scanInProgress       atomic.Bool
	autoScanStop         chan struct{}
	autoScanStopOnce     sync.Once
	thumbHighPriority    chan thumbJob
	thumbLowPriority     chan thumbJob
	thumbWorkerStop      chan struct{}
	thumbWorkerStopOnce  sync.Once
	thumbFocusID         atomic.Int64
	thumbBackgroundToken atomic.Int64
}

type thumbRow struct {
	widget.BaseWidget

	image    *canvas.Image
	name     *widget.Label
	selected *selectionBadge
	error    *widget.Label
}

type selectionBadge struct {
	widget.BaseWidget
	selected bool
}

type selectionBadgeRenderer struct {
	badge   *selectionBadge
	circle  *canvas.Circle
	check   *canvas.Text
	objects []fyne.CanvasObject
}

func newThumbRow() *thumbRow {
	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(92, 70))

	row := &thumbRow{
		image:    img,
		name:     widget.NewLabel(""),
		selected: newSelectionBadge(),
		error:    widget.NewLabel(""),
	}
	row.name.Truncation = fyne.TextTruncateEllipsis
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

func newSelectionBadge() *selectionBadge {
	badge := &selectionBadge{}
	badge.ExtendBaseWidget(badge)
	return badge
}

func (badge *selectionBadge) SetSelected(selected bool) {
	if badge.selected == selected {
		return
	}
	badge.selected = selected
	badge.Refresh()
}

func (badge *selectionBadge) CreateRenderer() fyne.WidgetRenderer {
	circle := canvas.NewCircle(color.Transparent)
	circle.StrokeWidth = 2
	check := canvas.NewText("✓", color.White)
	check.Alignment = fyne.TextAlignCenter
	check.TextStyle = fyne.TextStyle{Bold: true}
	check.TextSize = 17

	renderer := &selectionBadgeRenderer{
		badge:   badge,
		circle:  circle,
		check:   check,
		objects: []fyne.CanvasObject{circle, check},
	}
	renderer.Refresh()
	return renderer
}

func (renderer *selectionBadgeRenderer) Layout(size fyne.Size) {
	const badgeSize float32 = 26
	const checkHeight float32 = 22
	x := (size.Width - badgeSize) / 2
	y := (size.Height - badgeSize) / 2
	renderer.circle.Move(fyne.NewPos(x, y))
	renderer.circle.Resize(fyne.NewSize(badgeSize, badgeSize))
	renderer.check.Move(fyne.NewPos(x, y+(badgeSize-checkHeight)/2-1))
	renderer.check.Resize(fyne.NewSize(badgeSize, checkHeight))
}

func (renderer *selectionBadgeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(42, 70)
}

func (renderer *selectionBadgeRenderer) Refresh() {
	if renderer.badge.selected {
		renderer.circle.FillColor = color.NRGBA{R: 28, G: 142, B: 80, A: 255}
		renderer.circle.StrokeColor = color.NRGBA{R: 23, G: 116, B: 67, A: 255}
		renderer.check.Show()
	} else {
		renderer.circle.FillColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		renderer.circle.StrokeColor = color.NRGBA{R: 160, G: 166, B: 176, A: 255}
		renderer.check.Hide()
	}
	renderer.circle.Refresh()
	renderer.check.Refresh()
}

func (renderer *selectionBadgeRenderer) Objects() []fyne.CanvasObject {
	return renderer.objects
}

func (renderer *selectionBadgeRenderer) Destroy() {}

func main() {
	previewCacheDir, _ := os.MkdirTemp("", "photochoser-previews-*")
	if previewCacheDir != "" {
		defer os.RemoveAll(previewCacheDir)
	}

	fyneApp := app.NewWithID("com.photochoser.desktop")
	fyneApp.Settings().SetTheme(theme.LightTheme())

	w := fyneApp.NewWindow("PhotoChoser")
	w.Resize(fyne.NewSize(1180, 760))

	ui := &photoApp{
		window:            w,
		recursive:         true,
		current:           -1,
		transferMode:      photos.TransferMove,
		previewCacheDir:   previewCacheDir,
		thumbs:            newImageCache(thumbMemoryLimit),
		previewImages:     newImageCache(previewMemoryLimit),
		autoScanStop:      make(chan struct{}),
		thumbHighPriority: make(chan thumbJob, thumbQueueBuffer),
		thumbLowPriority:  make(chan thumbJob, thumbQueueBuffer),
		thumbWorkerStop:   make(chan struct{}),
	}
	ui.thumbFocusID.Store(-1)
	ui.build()
	ui.bindKeys()
	ui.startAutoScan()
	ui.startThumbWorkers()
	fyneApp.Lifecycle().SetOnEnteredForeground(func() {
		ui.restoreKeyboardFocus()
		ui.scanSilently()
	})
	fyneApp.Lifecycle().SetOnStopped(func() {
		ui.autoScanStopOnce.Do(func() {
			close(ui.autoScanStop)
		})
		ui.thumbWorkerStopOnce.Do(func() {
			close(ui.thumbWorkerStop)
		})
	})

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
	refreshButton := widget.NewButtonWithIcon("刷新", theme.ViewRefreshIcon(), ui.scan)
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
			row.selected.SetSelected(item.Selected)
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
		ui.restoreKeyboardFocus()
	}

	top := container.NewVBox(
		container.NewHBox(importButton, sourceButton, refreshButton, ui.sourceLabel, recursiveCheck),
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
	canvas.SetOnKeyDown(ui.handleKey)
	ui.restoreKeyboardFocus()
}

func (ui *photoApp) handleKey(event *fyne.KeyEvent) {
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
}

func (ui *photoApp) restoreKeyboardFocus() {
	if ui.window == nil {
		return
	}
	ui.window.Canvas().Unfocus()
}

func (ui *photoApp) scan() {
	ui.scanDirectory(false)
}

func (ui *photoApp) scanSilently() {
	ui.scanDirectory(true)
}

func (ui *photoApp) scanDirectory(silent bool) {
	if ui.sourceDir == "" {
		return
	}
	if !ui.scanInProgress.CompareAndSwap(false, true) {
		if !silent {
			ui.statusLabel.SetText("正在扫描照片...")
		}
		return
	}

	sourceDir := ui.sourceDir
	recursive := ui.recursive
	if !silent {
		ui.statusLabel.SetText("正在扫描照片...")
	}

	go func() {
		items, err := photos.Scan(sourceDir, recursive)
		fyne.Do(func() {
			defer ui.scanInProgress.Store(false)
			if sourceDir != ui.sourceDir || recursive != ui.recursive {
				return
			}
			if err != nil {
				if silent {
					ui.statusLabel.SetText("照片目录暂不可用，重新插入存储卡后会自动刷新")
				} else {
					ui.statusLabel.SetText(err.Error())
				}
				return
			}

			currentPath := ui.currentPath()
			currentID := ui.current
			added := countNewItems(items, ui.items)
			items = mergeScanProgress(items, ui.items)
			if samePhotoPaths(items, ui.items) {
				ui.items = items
				ui.list.Refresh()
				ui.refreshStatus()
			} else {
				ui.loadItemsKeepingPosition(items, currentPath, currentID, false)
			}
			if added > 0 {
				ui.statusLabel.SetText(fmt.Sprintf("已刷新，新增 %d 张，已选 %d 张", added, selectedCount(ui.items)))
			} else if !silent {
				ui.statusLabel.SetText(fmt.Sprintf("已刷新，%d 张 / 已选 %d 张", len(ui.items), selectedCount(ui.items)))
			}
		})
	}()
}

func (ui *photoApp) startAutoScan() {
	ticker := time.NewTicker(autoScanInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fyne.Do(ui.scanSilently)
			case <-ui.autoScanStop:
				return
			}
		}
	}()
}

func (ui *photoApp) currentPath() string {
	if ui.current < 0 || ui.current >= len(ui.items) {
		return ""
	}
	return ui.items[ui.current].Path
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
	ui.loadItemsKeepingPosition(items, "", 0, true)
}

func (ui *photoApp) loadItemsKeepingPosition(items []photos.Photo, preferredPath string, fallbackID int, clearImages bool) {
	ui.items = items
	ui.current = -1
	if clearImages {
		ui.clearImageCaches()
	}
	ui.errors = sync.Map{}
	ui.loading = sync.Map{}
	ui.previewLoading = sync.Map{}
	ui.thumbPreloadStarted.Store(false)
	ui.scanToken.Add(1)
	ui.list.Refresh()
	if len(items) > 0 {
		id := preferredItemID(items, preferredPath, fallbackID)
		ui.list.Select(id)
		if ui.current != id {
			ui.setCurrent(id)
		}
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
	if ui.prunePreviewImages(id) {
		ui.releaseUnusedMemorySoon()
	}

	token := ui.previewToken.Add(1)
	if imgValue, ok := ui.previewImages.Load(item.Path); ok {
		ui.mainImage.Image = imgValue
		ui.mainImage.Refresh()
		ui.refreshStatus()
		ui.preloadNearbyPreviews(id, ui.scanToken.Load(), token)
		ui.preloadThumbsOnce(ui.scanToken.Load())
		return
	}

	ui.showFastPreviewPlaceholder(item)
	ui.statusLabel.SetText("正在加载大图...")
	go ui.loadCurrentPreview(id, item.Path, item.Name, item.Selected, token, ui.scanToken.Load())
}

func (ui *photoApp) loadThumb(id int, token int64) {
	ui.prioritizeVisibleThumbs(id, token)
}

func (ui *photoApp) prioritizeVisibleThumbs(id int, token int64) {
	if id < 0 {
		return
	}
	ui.thumbFocusID.Store(int64(id))
	backgroundToken := ui.thumbBackgroundToken.Add(1)

	total := len(ui.items)
	for _, thumbID := range thumbPriorityOrder(total, id, thumbVisibleRadius) {
		ui.queueThumb(thumbJob{id: thumbID, token: token}, true)
	}
	ui.queueBackgroundThumbs(total, id, token, backgroundToken)
}

func (ui *photoApp) queueThumb(job thumbJob, highPriority bool) {
	if job.id < 0 {
		return
	}
	if highPriority {
		select {
		case ui.thumbHighPriority <- job:
		default:
			go ui.loadThumbNow(job)
		}
		return
	}
	select {
	case ui.thumbLowPriority <- job:
	default:
	}
}

func (ui *photoApp) startThumbWorkers() {
	highWorkers := max(1, thumbWorkerLimit-1)
	for range highWorkers {
		go ui.runHighThumbWorker()
	}
	for range max(1, thumbWorkerLimit-highWorkers) {
		go ui.runMixedThumbWorker()
	}
}

func (ui *photoApp) runHighThumbWorker() {
	for {
		select {
		case job := <-ui.thumbHighPriority:
			ui.loadThumbNow(job)
		case <-ui.thumbWorkerStop:
			return
		}
	}
}

func (ui *photoApp) runMixedThumbWorker() {
	for {
		select {
		case job := <-ui.thumbHighPriority:
			ui.loadThumbNow(job)
			continue
		default:
		}

		select {
		case job := <-ui.thumbHighPriority:
			ui.loadThumbNow(job)
		case job := <-ui.thumbLowPriority:
			ui.loadThumbNow(job)
		case <-ui.thumbWorkerStop:
			return
		}
	}
}

func (ui *photoApp) queueBackgroundThumbs(total int, center int, scanToken int64, backgroundToken int64) {
	if total == 0 {
		return
	}
	go func() {
		for _, id := range thumbPreloadOrder(total, center) {
			if ui.scanToken.Load() != scanToken || ui.thumbBackgroundToken.Load() != backgroundToken {
				return
			}
			ui.queueThumb(thumbJob{id: id, token: scanToken, backgroundToken: backgroundToken}, false)
		}
		if ui.scanToken.Load() == scanToken && ui.thumbBackgroundToken.Load() == backgroundToken {
			ui.releaseUnusedMemorySoon()
		}
	}()
}

func (ui *photoApp) loadThumbNow(job thumbJob) {
	if ui.shouldSkipThumbJob(job) {
		return
	}
	id := job.id
	token := job.token
	if id < 0 || id >= len(ui.items) {
		return
	}
	item := ui.items[id]
	if ui.shouldSkipThumbJob(job) {
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
			if ui.thumbs.Store(item.Path, img) {
				ui.releaseUnusedMemorySoon()
			}
		}
		if id < len(ui.items) {
			ui.list.RefreshItem(id)
		}
	})
}

func (ui *photoApp) shouldSkipThumbJob(job thumbJob) bool {
	if ui.scanToken.Load() != job.token {
		return true
	}
	if job.backgroundToken != 0 && ui.thumbBackgroundToken.Load() != job.backgroundToken {
		return true
	}
	if job.backgroundToken == 0 {
		focusID := int(ui.thumbFocusID.Load())
		if focusID >= 0 && abs(job.id-focusID) > thumbStaleDistance {
			return true
		}
	}
	return false
}

func (ui *photoApp) preloadThumbs(token int64) {
	total := len(ui.items)
	if total == 0 {
		return
	}
	current := ui.current
	backgroundToken := ui.thumbBackgroundToken.Add(1)
	ui.queueBackgroundThumbs(total, current, token, backgroundToken)
}

func (ui *photoApp) preloadThumbsOnce(token int64) {
	if ui.thumbPreloadStarted.CompareAndSwap(false, true) {
		ui.preloadThumbs(token)
	}
}

func (ui *photoApp) loadThumbImage(path string) (image.Image, error) {
	return preview.LoadCachedScaled(path, thumbMaxSide, ui.previewCacheDir)
}

func (ui *photoApp) showFastPreviewPlaceholder(item photos.Photo) {
	if imgValue, ok := ui.thumbs.Load(item.Path); ok {
		ui.mainImage.Image = imgValue
		ui.mainImage.Refresh()
		return
	}
	ui.mainImage.Image = nil
	ui.mainImage.Refresh()
}

func (ui *photoApp) loadCurrentPreview(id int, path string, name string, selected bool, token int64, scanToken int64) {
	time.Sleep(previewLoadWait)
	if ui.previewToken.Load() != token || ui.scanToken.Load() != scanToken {
		return
	}

	img, err := ui.loadPreviewImageShared(path)
	fyne.Do(func() {
		if ui.previewToken.Load() != token || ui.scanToken.Load() != scanToken {
			return
		}
		if err != nil {
			ui.mainImage.Image = nil
			ui.mainImage.Refresh()
			ui.statusLabel.SetText("无法预览：" + err.Error())
			return
		}
		if ui.previewImages.Store(path, img) {
			ui.releaseUnusedMemorySoon()
		}
		ui.mainImage.Image = img
		ui.mainImage.Refresh()
		ui.titleLabel.SetText(selectionMark(selected) + name)
		ui.refreshStatus()
		ui.preloadNearbyPreviews(id, scanToken, token)
		ui.preloadThumbsOnce(scanToken)
	})
}

func (ui *photoApp) preloadNearbyPreviews(current int, scanToken int64, previewToken int64) {
	ids := make([]int, 0, previewPrefetchNext+1)
	for offset := 1; offset <= previewPrefetchNext; offset++ {
		ids = append(ids, current+offset)
	}
	ids = append(ids, current-1)

	go func() {
		time.Sleep(previewPrefetchWait)
		for _, id := range ids {
			if ui.scanToken.Load() != scanToken || ui.previewToken.Load() != previewToken {
				return
			}
			ui.preloadPreview(id, scanToken, previewToken)
		}
	}()
}

func (ui *photoApp) preloadPreview(id int, scanToken int64, previewToken int64) {
	if id < 0 || id >= len(ui.items) || ui.scanToken.Load() != scanToken || ui.previewToken.Load() != previewToken {
		return
	}
	item := ui.items[id]
	if _, ok := ui.previewImages.Load(item.Path); ok {
		return
	}
	img, err := ui.loadPreviewImageShared(item.Path)
	if err != nil || ui.scanToken.Load() != scanToken || ui.previewToken.Load() != previewToken {
		return
	}
	if ui.previewImages.Store(item.Path, img) {
		ui.releaseUnusedMemorySoon()
	}
}

func (ui *photoApp) loadPreviewImageShared(path string) (image.Image, error) {
	job := &previewJob{done: make(chan struct{})}
	actual, loaded := ui.previewLoading.LoadOrStore(path, job)
	if loaded {
		existing := actual.(*previewJob)
		<-existing.done
		return existing.result.img, existing.result.err
	}
	defer ui.previewLoading.Delete(path)
	defer close(job.done)

	job.result.img, job.result.err = ui.loadPreviewImage(path)
	return job.result.img, job.result.err
}

func (ui *photoApp) loadPreviewImage(path string) (image.Image, error) {
	return preview.LoadCachedScaled(path, previewMaxSide, ui.previewCacheDir)
}

func (ui *photoApp) prunePreviewImages(current int) bool {
	keep := make(map[string]struct{}, previewPrefetchNext+2)
	for id := current - 1; id <= current+previewPrefetchNext; id++ {
		if id >= 0 && id < len(ui.items) {
			keep[ui.items[id].Path] = struct{}{}
		}
	}
	return ui.previewImages.DeleteExcept(keep)
}

func (ui *photoApp) clearImageCaches() {
	cleared := false
	if ui.thumbs == nil {
		ui.thumbs = newImageCache(thumbMemoryLimit)
	} else if ui.thumbs.Clear() {
		cleared = true
	}
	if ui.previewImages == nil {
		ui.previewImages = newImageCache(previewMemoryLimit)
	} else if ui.previewImages.Clear() {
		cleared = true
	}
	if cleared {
		ui.releaseUnusedMemorySoon()
	}
}

func (ui *photoApp) releaseUnusedMemorySoon() {
	if !ui.memoryTrimPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		time.Sleep(memoryTrimDelay)
		runtime.GC()
		debug.FreeOSMemory()
		ui.memoryTrimPending.Store(false)
	}()
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

func mergeScanProgress(scanned []photos.Photo, existing []photos.Photo) []photos.Photo {
	selected := make(map[string]bool, len(existing))
	for _, item := range existing {
		if item.Selected {
			selected[item.Path] = true
		}
	}
	for i := range scanned {
		if selected[scanned[i].Path] {
			scanned[i].Selected = true
		}
	}
	return scanned
}

func countNewItems(scanned []photos.Photo, existing []photos.Photo) int {
	known := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		known[item.Path] = struct{}{}
	}
	count := 0
	for _, item := range scanned {
		if _, ok := known[item.Path]; !ok {
			count++
		}
	}
	return count
}

func samePhotoPaths(a []photos.Photo, b []photos.Photo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path {
			return false
		}
	}
	return true
}

func thumbPreloadOrder(total int, current int) []int {
	if total <= 0 {
		return nil
	}
	if current < 0 || current >= total {
		current = 0
	}

	order := make([]int, 0, total)
	order = append(order, current)
	for distance := 1; len(order) < total; distance++ {
		next := current + distance
		if next < total {
			order = append(order, next)
		}
		previous := current - distance
		if previous >= 0 {
			order = append(order, previous)
		}
	}
	return order
}

func thumbPriorityOrder(total int, current int, radius int) []int {
	order := thumbPreloadOrder(total, current)
	if len(order) == 0 {
		return nil
	}
	limit := radius*2 + 1
	if limit < 1 {
		limit = 1
	}
	if limit > len(order) {
		limit = len(order)
	}
	return order[:limit]
}

func preferredItemID(items []photos.Photo, preferredPath string, fallbackID int) int {
	if preferredPath != "" {
		for i, item := range items {
			if item.Path == preferredPath {
				return i
			}
		}
	}
	if fallbackID < 0 {
		return 0
	}
	if fallbackID >= len(items) {
		return len(items) - 1
	}
	return fallbackID
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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
