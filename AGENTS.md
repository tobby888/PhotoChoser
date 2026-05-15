# PhotoChoser Codex Guide

## Quick Run

- Run the app: `make run`
- Run tests: `make test`
- Build local binary: `make build`
- Check Windows-only packages from macOS: `make check-windows`

The Makefile keeps Go caches inside `.cache/` so Codex sandboxed runs do not write to the user-level Go cache.

## Notes

- This is a Go/Fyne desktop app.
- The main entry point is `cmd/photochoser/main.go`.
- RAW preview code lives in `internal/preview`.
- Photo scanning, filtering, and moving code lives in `internal/photos`.

