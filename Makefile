GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/mod
GOENV = GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)

.PHONY: run test build build-mac package-macos dmg check-windows clean

run:
	$(GOENV) go run ./cmd/photochoser

test:
	$(GOENV) go test ./...

build:
	$(GOENV) go build -o bin/photochoser ./cmd/photochoser

build-mac:
	$(GOENV) go build -o bin/PhotoChoser ./cmd/photochoser

package-macos:
	./scripts/package_macos.sh

dmg: package-macos

check-windows:
	GOOS=windows $(GOENV) go test -c -o /private/tmp/photochoser-nativepicker-win.test.exe ./internal/nativepicker
	GOOS=windows $(GOENV) go test -c -o /private/tmp/photochoser-preview-win.test.exe ./internal/preview

clean:
	rm -rf bin dist
