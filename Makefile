GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/mod
PHOTOCHOSER_GOPATH := $(CURDIR)/.cache/go

ifeq ($(OS),Windows_NT)
GOENV = set "GOPATH=$(PHOTOCHOSER_GOPATH)"&& set "GOCACHE=$(GOCACHE)"&& set "GOMODCACHE=$(GOMODCACHE)"&&
GOWINENV = set "GOOS=windows"&& $(GOENV)
MKDIR_BIN = if not exist bin mkdir bin
CHECK_WINDOWS_DIR = $(CURDIR)/.cache/check-windows
MKDIR_CHECK_WINDOWS = if not exist "$(CHECK_WINDOWS_DIR)" mkdir "$(CHECK_WINDOWS_DIR)"
RM_ARTIFACTS = if exist bin rmdir /s /q bin & if exist dist rmdir /s /q dist
else
GOENV = GOPATH=$(PHOTOCHOSER_GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
GOWINENV = GOOS=windows $(GOENV)
MKDIR_BIN = mkdir -p bin
CHECK_WINDOWS_DIR = /private/tmp
MKDIR_CHECK_WINDOWS = mkdir -p "$(CHECK_WINDOWS_DIR)"
RM_ARTIFACTS = rm -rf bin dist
endif

.PHONY: run test build build-mac build-windows package-macos dmg check-windows clean

run:
	$(GOENV) go run ./cmd/photochoser

test:
	$(GOENV) go test ./...

build:
	$(MKDIR_BIN)
	$(GOENV) go build -o bin/photochoser ./cmd/photochoser

build-mac:
	$(MKDIR_BIN)
	$(GOENV) go build -o bin/PhotoChoser ./cmd/photochoser

package-macos:
	./scripts/package_macos.sh

dmg: package-macos

build-windows:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-windows.ps1

check-windows:
	$(MKDIR_CHECK_WINDOWS)
	$(GOWINENV) go test -c -o "$(CHECK_WINDOWS_DIR)/photochoser-nativepicker-win.test.exe" ./internal/nativepicker
	$(GOWINENV) go test -c -o "$(CHECK_WINDOWS_DIR)/photochoser-preview-win.test.exe" ./internal/preview

clean:
	$(RM_ARTIFACTS)
