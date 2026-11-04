# Sweep - Modern Multi-Distro Linux System Cleaner
# Build & Development Makefile

SHELL := /bin/bash
BIN_DIR := bin
BINARY_NAME := sweep
GUI_BINARY_NAME := sweep-gui
TARGET := $(BIN_DIR)/$(BINARY_NAME)
GUI_TARGET := $(BIN_DIR)/$(GUI_BINARY_NAME)

PREFIX ?= /usr/local
DESTDIR ?=
BINDIR ?= $(PREFIX)/bin
SHAREDIR ?= /usr/share

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0-beta")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

GO ?= go
GOPATH ?= $(shell $(GO) env GOPATH)
NFPM ?= $(shell command -v nfpm 2>/dev/null || (test -x "$(GOPATH)/bin/nfpm" && echo "$(GOPATH)/bin/nfpm") || echo "nfpm")

.PHONY: all build build-gui build-all build-frontend test lint vet fmt fmt-check clean run cross-compile package install uninstall validate-desktop help

all: build

## build: Compile the sweep CLI/TUI binary with version ldflags
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(TARGET) ./cmd/sweep
	@echo "Build complete: $(TARGET)"

## build-gui: Compile the desktop GUI binary with WebKit2GTK support
build-gui:
	@if [ ! -d "cmd/sweep-gui/frontend/dist" ]; then \
		echo "Frontend dist not found, building frontend..."; \
		$(MAKE) build-frontend; \
	fi
	@mkdir -p $(BIN_DIR)
	$(GO) build -tags "production,webkit2_41" -ldflags "$(LDFLAGS)" -o $(GUI_TARGET) ./cmd/sweep-gui
	@echo "GUI build complete: $(GUI_TARGET)"

## build-all: Compile both CLI/TUI and Desktop GUI binaries
build-all: build build-gui
	@echo "Build all complete: $(TARGET) and $(GUI_TARGET)"

## build-frontend: Build frontend web assets via pnpm and sync to cmd/sweep-gui
build-frontend:
	cd frontend && pnpm install --frozen-lockfile && pnpm build
	@echo "Frontend build and sync complete."

## test: Run all unit and integration tests with race detection
test:
	$(GO) test -v -race ./...

## lint: Run go vet and code format checks
lint: vet fmt-check

## vet: Run go vet on all packages
vet:
	$(GO) vet ./...

## fmt: Auto-format all Go source files
fmt:
	gofmt -s -w .

## fmt-check: Check if all Go source files adhere to gofmt standard
fmt-check:
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || (echo "Formatting issues detected. Run 'make fmt' to fix." && exit 1)

## validate-desktop: Validate the freedesktop .desktop entry
validate-desktop:
	@command -v desktop-file-validate >/dev/null 2>&1 || (echo "desktop-file-validate not found; skipping" && exit 0)
	desktop-file-validate build/linux/io.github.efzynx.sweep.desktop

## cross-compile: Cross-compile sweep CLI binaries for standard Linux architectures
cross-compile:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/sweep
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/sweep
	@echo "Cross-compilation complete: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64, $(BIN_DIR)/$(BINARY_NAME)-linux-arm64"

## package: Build .deb, .rpm, and Arch packages using nfpm
package: build-all
	@if ! command -v $(NFPM) >/dev/null 2>&1 && [ ! -x "$(NFPM)" ]; then \
		echo "nfpm not found. Run 'go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest' or add $(GOPATH)/bin to PATH."; \
		exit 1; \
	fi
	@mkdir -p staging dist
	@cp $(TARGET) staging/sweep
	@cp $(GUI_TARGET) staging/sweep-gui
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-cli.yaml --packager deb --target dist/
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-cli.yaml --packager rpm --target dist/
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-cli.yaml --packager archlinux --target dist/
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-gui.yaml --packager deb --target dist/
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-gui.yaml --packager rpm --target dist/
	@ARCH=amd64 VERSION=$(shell echo "$(VERSION)" | sed 's/^v//') $(NFPM) package --config build/package/nfpm-gui.yaml --packager archlinux --target dist/
	@rm -rf staging
	@echo "Packages generated in dist/:"
	@ls -lh dist/*.deb dist/*.rpm dist/*.pkg.tar.zst 2>/dev/null || true

## install: Install sweep binaries, .desktop entry, and icons to system directories
install:
	@install -d -m 755 $(DESTDIR)$(BINDIR)
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/applications
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/icons/hicolor/scalable/apps
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/icons/hicolor/128x128/apps
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/icons/hicolor/256x256/apps
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/icons/hicolor/512x512/apps
	@install -d -m 755 $(DESTDIR)$(SHAREDIR)/pixmaps
	@if [ -f $(TARGET) ]; then \
		install -m 755 $(TARGET) $(DESTDIR)$(BINDIR)/$(BINARY_NAME); \
		echo "Installed $(TARGET) -> $(DESTDIR)$(BINDIR)/$(BINARY_NAME)"; \
	fi
	@if [ -f $(GUI_TARGET) ]; then \
		install -m 755 $(GUI_TARGET) $(DESTDIR)$(BINDIR)/$(GUI_BINARY_NAME); \
		echo "Installed $(GUI_TARGET) -> $(DESTDIR)$(BINDIR)/$(GUI_BINARY_NAME)"; \
	fi
	@install -m 644 build/linux/io.github.efzynx.sweep.desktop $(DESTDIR)$(SHAREDIR)/applications/io.github.efzynx.sweep.desktop
	@install -m 644 build/linux/io.github.efzynx.sweep.svg $(DESTDIR)$(SHAREDIR)/icons/hicolor/scalable/apps/io.github.efzynx.sweep.svg
	@install -m 644 build/linux/io.github.efzynx.sweep-128x128.png $(DESTDIR)$(SHAREDIR)/icons/hicolor/128x128/apps/io.github.efzynx.sweep.png
	@install -m 644 build/linux/io.github.efzynx.sweep-256x256.png $(DESTDIR)$(SHAREDIR)/icons/hicolor/256x256/apps/io.github.efzynx.sweep.png
	@install -m 644 build/linux/io.github.efzynx.sweep-512x512.png $(DESTDIR)$(SHAREDIR)/icons/hicolor/512x512/apps/io.github.efzynx.sweep.png
	@install -m 644 build/linux/io.github.efzynx.sweep.png $(DESTDIR)$(SHAREDIR)/pixmaps/io.github.efzynx.sweep.png
	@echo "Installation complete."
	@if [ -z "$(DESTDIR)" ]; then \
		command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database $(SHAREDIR)/applications || true; \
		command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f -t $(SHAREDIR)/icons/hicolor || true; \
	fi

## uninstall: Remove installed sweep binaries, .desktop entry, and icons
uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	rm -f $(DESTDIR)$(BINDIR)/$(GUI_BINARY_NAME)
	rm -f $(DESTDIR)$(SHAREDIR)/applications/io.github.efzynx.sweep.desktop
	rm -f $(DESTDIR)$(SHAREDIR)/icons/hicolor/scalable/apps/io.github.efzynx.sweep.svg
	rm -f $(DESTDIR)$(SHAREDIR)/icons/hicolor/128x128/apps/io.github.efzynx.sweep.png
	rm -f $(DESTDIR)$(SHAREDIR)/icons/hicolor/256x256/apps/io.github.efzynx.sweep.png
	rm -f $(DESTDIR)$(SHAREDIR)/icons/hicolor/512x512/apps/io.github.efzynx.sweep.png
	rm -f $(DESTDIR)$(SHAREDIR)/pixmaps/io.github.efzynx.sweep.png
	@echo "Uninstall complete."

## clean: Remove compiled binaries and build artifacts
clean:
	rm -rf $(BIN_DIR)
	@echo "Clean complete."

## run: Build and run sweep in scan-only mode
run: build
	./$(TARGET) --scan-only

## help: Display this help message
help:
	@echo "Sweep Build System"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
