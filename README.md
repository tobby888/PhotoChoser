# PhotoChoser

PhotoChoser 是一个面向活动摄影快速返图的跨平台选片工具，使用 Go 开发，目标平台为 macOS 和 Windows。它的第一屏就是选片工作区：导入照片文件夹、浏览预览、用快捷键标记保留照片，然后把已选照片移动或复制到交付目录。

## 适合场景

- 婚礼、活动、会议、演出等需要当天快速初选和交付的拍摄。
- 从相机卡、移动硬盘或本地文件夹中快速浏览大量照片。
- RAW + XMP 工作流：处理 RAW 文件时自动带走同名 `.xmp` 或 `.XMP` 边车文件。
- 需要保留原始文件名，同时避免目标目录里同名文件被覆盖。

## 功能

- 支持系统原生文件夹选择，一次导入整个照片文件夹。
- 扫描照片目录时支持递归扫描。
- 重新扫描同一目录时会保留已挑选照片，并把新增照片加入列表；存储卡重新插入后也会自动刷新。
- 使用系统原生文件夹选择窗口选择照片目录和目标目录。
- 左侧显示缩略图列表，右侧显示大图预览。
- JPEG 和 RAW 内嵌 JPEG 预览会按 EXIF 横竖屏方向显示。
- 使用快捷键快速切换和挑选照片。
- 可自行选择将已挑选照片移动或复制到指定目录。
- 移动或复制 RAW 文件时会同时处理同名 `.xmp` 边车文件。
- 使用 Go 协程并发生成缩略图，并使用会话级临时缓存加快重复显示；软件退出时会清除预览缓存。
- 支持 JPEG、PNG、TIFF，以及主流微单/相机 RAW 文件扩展名。
- RAW 预览优先读取文件内嵌 JPEG 预览；发布用 Windows GUI exe 会静态内嵌 LibRaw，缺少内嵌预览时不需要用户安装系统 RAW 编解码支持。
- 自动忽略 macOS 在存储卡上生成的 `._*` AppleDouble 元数据文件，避免把它们误当作 RAW。

## 快速开始

开发环境中可以直接运行：

```bash
make run
```

也可以直接使用 Go 命令：

```bash
GOCACHE=$PWD/.cache/go-build GOMODCACHE=$PWD/.cache/mod go run ./cmd/photochoser
```

启动后会打开 PhotoChoser 窗口，左侧是缩略图列表，右侧是当前照片的大图预览。

## 使用方法

1. 点击 `导入文件夹`，选择相机卡或本地照片文件夹。
2. 按需勾选或取消 `包含子目录`。勾选后会递归扫描子文件夹；取消后只扫描当前文件夹。
3. 等待缩略图加载。第一张照片会自动显示在右侧大图预览区。
4. 使用鼠标点击左侧缩略图，或用 `←` / `→` 在照片之间切换。
5. 看到要保留的照片时，按 `Space` 标记为已选；再次按 `Space` 会取消标记。已选照片会显示 `✓`。
6. 点击 `目标目录`，选择交付文件夹。
7. 在 `移动` 和 `复制` 中选择交付方式。默认是 `移动`。
8. 按 `M`、`Enter`，或点击 `移动已选` / `复制已选`，把所有已选照片交付到目标目录。

交付完成后，状态栏会显示处理数量。如果选择 `移动`，已移动的照片会从当前列表中移除；如果选择 `复制`，源文件会保留在原目录，已复制照片会自动取消勾选。如果目标目录里已有同名文件，PhotoChoser 会自动追加 `_001`、`_002` 这样的后缀，避免覆盖已有文件。

## 快捷键

- `←` / `→`：上一张 / 下一张
- `Space`：挑选或取消挑选当前照片
- `M` 或 `Enter`：按当前选择的 `移动` 或 `复制` 方式处理所有已挑选照片

## 导入和扫描

`导入文件夹` 和 `扫描目录` 都会打开系统原生文件夹选择窗口，用于选择要扫描的照片目录。PhotoChoser 会过滤不支持的文件，只把照片和 RAW 文件放入列表。

默认会启用 `包含子目录`，适合相机卡里按日期、机身或相机自动分目录的情况。如果你只想处理当前目录里的照片，可以取消勾选。

如果拍摄中途需要拔出存储卡继续拍摄，PhotoChoser 会保留当前列表和已选标记。卡重新插入、目录重新可用后，软件会自动刷新并补进新增照片；也可以点击 `刷新` 立即重新扫描，已有选择会按原文件路径保留。

扫描完成后，PhotoChoser 会用多个 Go 协程并发预生成缩略图。缩略图和大图预览会写入本次运行的临时缓存目录，方便列表滚动、切换照片和重新显示时复用；退出软件时缓存目录会被删除。

macOS 可能会在外接盘或相机卡中生成 `._DSC0001.ARW` 这类 AppleDouble 元数据文件。它们不是照片，PhotoChoser 会自动忽略。

## 预览行为

JPEG、PNG、TIFF 会直接解码显示。JPEG 和 RAW 内嵌 JPEG 预览会根据 EXIF/TIFF 方向信息自动旋转，避免竖拍照片横着显示。

切换当前照片时，PhotoChoser 会优先显示已有缩略图或已缓存的大图，随后在后台生成更清晰的大图预览。它还会提前预取当前照片后面的几张和前一张大图预览，让连续按方向键选片更顺滑。

RAW 文件会优先读取相机写入文件里的内嵌 JPEG 预览，这比完整 RAW 解码更快，适合选片。发布用 Windows GUI exe 使用静态内嵌的 LibRaw 作为兜底解码后端；如果没有找到内嵌 JPEG，PhotoChoser 会直接在应用内部解码 RAW，不要求用户安装 Windows RAW Image Extension 或相机厂商 codec。

## RAW 格式支持

当前版本会识别并尝试预览以下 RAW 扩展名：

`.arw`, `.srf`, `.sr2`, `.nef`, `.nrw`, `.cr2`, `.cr3`, `.crw`, `.raf`, `.rw2`, `.orf`, `.dng`, `.pef`, `.3fr`, `.fff`, `.iiq`, `.mos`, `.mrw`, `.x3f`, `.kdc`, `.erf`, `.mef`, `.rwl`

这些扩展名覆盖 Sony、Nikon、Canon、Fujifilm、Panasonic、Olympus / OM System、Pentax、Leica、Hasselblad、Phase One、Sigma、Kodak、Epson、Mamiya 等常见相机系统。Sony `.arw`，包括常见索尼微单压缩 ARW，也会被识别并尝试预览。

## 交付规则

- `移动` 会移动照片原文件，成功后从当前列表移除。
- `复制` 会把照片复制到目标目录，源文件保留在原目录，成功后清除这些照片的已选状态。
- 同名目标文件不会被覆盖，会自动生成带编号后缀的新文件名，例如 `DSC00123_001.ARW`。
- 处理 RAW 文件时，如果旁边有同名 `.xmp` 或 `.XMP` 文件，会一起移动或复制到目标目录。
- 使用 `移动` 且源目录和目标目录不在同一个磁盘或卷上时，PhotoChoser 会在内部复制后删除源文件，以完成跨盘移动。

## 常用开发命令

```bash
make test
make build
make build-windows
make package-windows
make check-windows
make package-macos
```

`Makefile` 会把 Go 缓存放到项目内的 `.cache/` 目录，便于在沙盒或 CI 环境中运行。

## 本地构建

```bash
make build
```

构建产物会输出到 `bin/photochoser`。

在 Windows 上构建不会显示黑框的 GUI 程序：

```powershell
$env:LIBRAW_DIR="C:\path\to\libraw-static"
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

也可以使用：

```bash
make build-windows
```

Windows 构建产物会输出到 `bin/PhotoChoser.exe`。该构建会启用 `-tags=libraw` 和 `-ldflags="-H=windowsgui"`，从资源管理器双击启动时不会显示控制台黑框。脚本要求 `LIBRAW_DIR` 指向静态 LibRaw 安装根目录，或者手动提供 `CGO_CFLAGS` 和 `CGO_LDFLAGS`；这样最终产物仍是单个 GUI `.exe`，不随包分发 DLL 或 helper。

## 打包

Windows 上可以生成用于发布的 zip 包：

```powershell
$env:LIBRAW_DIR="C:\path\to\libraw-static"
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\package_windows.ps1
```

也可以使用：

```bash
make package-windows
```

构建产物会输出到 `dist/release/PhotoChoser-<version>-windows-amd64.zip`，压缩包内只包含单个 `PhotoChoser.exe` GUI 应用。

macOS 上可以直接生成可运行的 `.app` 和可分发的 `.dmg`：

```bash
make package-macos
```

构建产物会输出到：

- `dist/macos/PhotoChoser.app`
- `dist/PhotoChoser-macos.dmg`
- `dist/release/PhotoChoser-<version>-macos-<arch>.dmg`

`PhotoChoser.app` 可以直接双击运行；`PhotoChoser-macos.dmg` 打开后可将 PhotoChoser 拖入 Applications。脚本使用系统自带的 `hdiutil` 制作 DMG，并对 app bundle 做 ad-hoc 签名，方便本机运行和测试。

如需覆盖 app 版本号，可在打包时传入 `APP_VERSION` 和 `BUILD_VERSION`：

```bash
APP_VERSION=1.0.0 BUILD_VERSION=100 make package-macos
```

## GitHub Releases

推送 `v*` 标签会自动触发 GitHub Actions 构建 Windows 和 macOS 版本，并把产物上传到对应的 GitHub Release：

```bash
git tag v0.1.0
git push origin v0.1.0
```

也可以在 GitHub Actions 页面手动运行 `Release` workflow，并填写版本号。自动发布的产物包括：

- `PhotoChoser-<version>-windows-amd64.zip`
- `PhotoChoser-<version>-macos-<arch>.dmg`

如果发布步骤报 `403 Resource not accessible by integration`，说明 GitHub Actions 的默认 token 没有创建 release 的写权限。到仓库的 `Settings` -> `Actions` -> `General` -> `Workflow permissions`，选择 `Read and write permissions`，保存后重新运行失败的 `Release` workflow。
