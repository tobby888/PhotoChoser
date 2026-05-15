# PhotoChoser

PhotoChoser 是一个面向活动摄影快速返图的跨平台选片工具，使用 Go 开发，目标平台为 macOS 和 Windows。

## 功能

- 支持系统原生文件夹选择，一次导入整个照片文件夹。
- 扫描照片目录时支持递归扫描。
- 使用系统原生文件夹选择窗口选择照片目录和目标目录。
- 左侧显示缩略图列表，右侧显示大图预览。
- JPEG 和 RAW 内嵌 JPEG 预览会按 EXIF 横竖屏方向显示。
- 使用快捷键快速切换和挑选照片。
- 将已挑选照片移动到指定目录。
- 移动 RAW 文件时会同时移动同名 `.xmp` 边车文件。
- 支持 JPEG、PNG、TIFF，以及主流微单/相机 RAW 文件扩展名。
- RAW 预览优先读取文件内嵌 JPEG 预览，适合快速选片场景，也兼容索尼压缩 RAW 等常见格式的快速查看。
- 自动忽略 macOS 在存储卡上生成的 `._*` AppleDouble 元数据文件，避免把它们误当作 RAW。

## 快捷键

- `←` / `→`：上一张 / 下一张
- `Space`：挑选或取消挑选当前照片
- `M` 或 `Enter`：移动所有已挑选照片到目标目录

## RAW 格式支持

当前版本会识别并尝试预览以下 RAW 扩展名：

`.arw`, `.srf`, `.sr2`, `.nef`, `.nrw`, `.cr2`, `.cr3`, `.crw`, `.raf`, `.rw2`, `.orf`, `.dng`, `.pef`, `.3fr`, `.fff`, `.iiq`, `.mos`, `.mrw`, `.x3f`, `.kdc`, `.erf`, `.mef`, `.rwl`

为了速度，软件会先读取 RAW 文件中的相机内嵌 JPEG 预览；如果没有找到，会尝试调用系统原生 RAW 解码能力作为兜底。这通常是活动摄影快速选片最合适的路径。

## 开发运行

```bash
make run
```

也可以直接使用 Go 命令：

```bash
GOCACHE=$PWD/.cache/go-build GOMODCACHE=$PWD/.cache/mod go run ./cmd/photochoser
```

## 常用开发命令

```bash
make test
make build
make check-windows
```

## 打包

安装 Fyne CLI 后可以分别在 macOS 和 Windows 上打包：

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin -icon Icon.png
fyne package -os windows -icon Icon.png
```
