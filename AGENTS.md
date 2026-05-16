# PhotoChoser Codex Guide

## Project Goal

PhotoChoser is a Go desktop app for event photographers who need fast same-day photo selection and delivery. The app must help the photographer quickly import photos, view thumbnails and large previews, mark keepers with shortcuts, and move or copy selected photos into a chosen delivery folder.

## User Requirements

- Build with Go and keep the project cross-platform for macOS and Windows.
- Windows GUI builds must use the Windows GUI subsystem so launching the `.exe` does not show a console window.
- Keep the project in git and commit completed changes.
- The first-screen experience must be the usable photo selection app, not a landing page or demo shell.
- Support fast viewing of many event photos from camera cards and local folders.
- Support system-native folder selection so the user imports a whole photo folder, not individual photo files.
- Keep directory scanning available, including recursive scanning.
- Preserve current selections when rescanning the same source folder, and refresh automatically when a temporarily removed camera card or source folder becomes available again.
- Use system-native folder selection for choosing scan folders and target folders; do not use Fyne's internal folder picker for this.
- Show a thumbnail list and a large preview for the current photo.
- Generate thumbnails concurrently with Go goroutines for fast browsing; cache and prefetch large previews to keep keyboard navigation responsive. Session preview caches are allowed, but all cache files must be cleared when the app exits.
- Rotate JPEG previews according to EXIF/TIFF orientation metadata before displaying them.
- Use shortcuts for fast culling: left/right to navigate, space to toggle selection, `M` or Enter to run the selected move/copy delivery action.
- Let the photographer choose whether selected photos are moved or copied to the target folder.
- Move or copy all selected photos to the target folder, preserving filenames and avoiding overwrite collisions with suffixes.
- Move or copy same-name `.xmp` or `.XMP` sidecar files together with RAW files.
- Support JPEG, PNG, TIFF, and mainstream mirrorless/camera RAW formats.
- RAW support must include vendor formats from Sony, Nikon, Canon, Fujifilm, Panasonic, Olympus/OM System, Pentax, Leica, Hasselblad, Phase One, Sigma, Kodak, Epson, Mamiya, and similar cameras.
- Sony `.arw` files, including compressed ARW from Sony mirrorless bodies, must be supported.
- Ignore macOS AppleDouble metadata files such as `._DSC07792.ARW`; these are not real RAW photos and must not appear in the photo list.

## RAW Preview Strategy

- For speed, prefer reading embedded JPEG previews from RAW files.
- If no embedded JPEG preview is found, use the OS-native RAW preview fallback.
- On macOS, the current fallback uses `sips` to convert RAW to a temporary JPEG.
- On Windows, the current fallback uses PowerShell/WIC to decode RAW when the system has the right codec support.
- Do not replace fast preview loading with full RAW demosaic processing unless the user explicitly asks; culling speed matters more than final RAW development accuracy.

## Current Architecture

- Main app entry point: `cmd/photochoser/main.go`
- Photo scanning, import filtering, sorting, and move/copy delivery: `internal/photos`
- RAW and image preview loading: `internal/preview`
- Native file/folder picker wrappers: `internal/nativepicker`
- Cross-platform build check: `.github/workflows/build.yml`
- Release workflow: `.github/workflows/release.yml`
- Project quick commands: `Makefile`
- Windows GUI build helper: `scripts/build-windows.ps1`
- Windows release package helper: `scripts/package_windows.ps1`

## Quick Run

- Run the app: `make run`
- Run tests: `make test`
- Build local binary: `make build`
- Build macOS app bundle and DMG: `make package-macos`
- Build Windows GUI binary without a console window: `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1`
- Build Windows release zip: `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\package_windows.ps1`
- Check Windows-only packages from macOS: `make check-windows`

The Makefile keeps Go caches inside `.cache/` so Codex sandboxed runs do not write to the user-level Go cache.

## macOS Packaging

- Use `make package-macos` on macOS to create both `dist/macos/PhotoChoser.app` and `dist/PhotoChoser-macos.dmg`.
- The packaging script is `scripts/package_macos.sh`; it does not require Fyne CLI.
- The app bundle is ad-hoc signed for local testing, but it is not notarized for public distribution.
- GitHub Releases are created by `.github/workflows/release.yml` when a `v*` tag is pushed or the workflow is run manually.

## Known Context

- The user tested Sony ARW files from a mounted card at paths like `/Volumes/Untitled/DCIM/100MSDCF`.
- A previous failure showed `._DSC07792.ARW`, which was confirmed to be `AppleDouble encoded Macintosh file`; the real file is `DSC07792.ARW`.
- `sips -s format jpeg /Volumes/Untitled/DCIM/100MSDCF/DSC07792.ARW --out /private/tmp/photochoser-arw-test.jpg` succeeded on macOS.
- macOS builds may print `ld: warning: ignoring duplicate libraries: '-lobjc'`; this warning has not blocked tests or builds.

## Implementation Preferences

- Prefer focused bug fixes over broad rewrites.
- Preserve the existing Fyne UI unless a bug or user request requires changing it.
- Keep user-facing text concise and suitable for a fast photography workflow.
- Avoid adding heavyweight RAW dependencies unless necessary; prefer native OS support and embedded previews first.
- Keep generated binaries and caches out of git.
- Update README and AGENTS.md when workflow or requirements change.

## Verification

Run these before handing off meaningful code changes:

```bash
make test
make build
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\package_windows.ps1
make check-windows
```

For changes that affect RAW preview on macOS, also test at least one real `.ARW` file with `sips` or through `make run` when a camera card is mounted.
