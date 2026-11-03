# Sweep - Modern Multi-Distro Linux System Cleaner
# Build & Development Makefile

SHELL := /bin/bash
BIN_DIR := bin
BINARY_NAME := sweep
TARGET := $(BIN_DIR)/$(BINARY_NAME)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

GO ?= go

.PHONY: all build test lint fmt fmt-check clean run cross-compile help

all: build

## build: Compile the sweep binary with version ldflags
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(TARGET) ./cmd/sweep
	@echo "Build complete: $(TARGET)"

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

## cross-compile: Cross-compile sweep binaries for standard Linux architectures
cross-compile:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/sweep
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/sweep
	@echo "Cross-compilation complete: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64, $(BIN_DIR)/$(BINARY_NAME)-linux-arm64"

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
