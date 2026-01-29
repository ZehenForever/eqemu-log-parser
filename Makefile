SHELL := /bin/sh

GO ?= go
NPM ?= npm
WAILS ?= wails

ROOT := $(CURDIR)
DIST := $(ROOT)/dist

EQLOG_PKG := ./cmd/eqlog
EQLOGHUB_PKG := ./cmd/eqloghub

EQLOGUI_DIR := $(ROOT)/cmd/eqlogui
EQLOGUI_FRONTEND_DIR := $(EQLOGUI_DIR)/frontend

HOST_OS := $(shell $(GO) env GOOS)
HOST_ARCH := $(shell $(GO) env GOARCH)

# Helper: add .exe on Windows
EXT_HOST :=
ifeq ($(HOST_OS),windows)
	EXT_HOST := .exe
endif

.PHONY: help clean test \
	build build-eqlog build-eqloghub build-eqlogui \
	build-all build-eqlog-all build-eqloghub-all \
	build-linux-amd64 build-eqlog-linux-amd64 build-eqloghub-linux-amd64 \
	build-eqlogui-wails build-eqlogui-wails-all

help:
	@echo "Targets:"
	@echo "  build                    Build eqlog, eqloghub, eqlogui for host platform (named with GOOS/GOARCH)"
	@echo "  build-all                Cross-build eqlog + eqloghub for win/linux/mac (amd64+arm64)"
	@echo "  build-linux-amd64         Build eqlog + eqloghub for linux/amd64 (DigitalOcean)"
	@echo "  build-eqlogui-wails       Build eqlogui via wails for host platform (requires wails + toolchain)"
	@echo "  build-eqlogui-wails-all   Attempt wails builds for windows/linux/darwin amd64 (toolchains required)"
	@echo "  clean                    Remove dist/"
	@echo "  test                     Run go tests (root + eqlogui module dir)"

clean:
	rm -rf "$(DIST)"

test:
	$(GO) test ./...
	$(GO) test ./... -C "$(EQLOGUI_DIR)"

$(DIST):
	mkdir -p "$(DIST)"

# -------------------------
# Host builds (suffix names)
# -------------------------

build: build-eqlog build-eqloghub build-eqlogui

build-eqlog: $(DIST)
	@echo "==> eqlog $(HOST_OS)/$(HOST_ARCH)"
	CGO_ENABLED=0 $(GO) build -trimpath -o "$(DIST)/eqlog_$(HOST_OS)_$(HOST_ARCH)$(EXT_HOST)" $(EQLOG_PKG)

build-eqloghub: $(DIST)
	@echo "==> eqloghub $(HOST_OS)/$(HOST_ARCH)"
	CGO_ENABLED=0 $(GO) build -trimpath -o "$(DIST)/eqloghub_$(HOST_OS)_$(HOST_ARCH)$(EXT_HOST)" $(EQLOGHUB_PKG)

build-eqlogui: $(DIST)
	@echo "==> eqlogui (frontend) $(HOST_OS)/$(HOST_ARCH)"
	$(NPM) run build -C "$(EQLOGUI_FRONTEND_DIR)"
	@echo "==> eqlogui (backend) $(HOST_OS)/$(HOST_ARCH)"
	CGO_ENABLED=0 $(GO) build -trimpath -o "$(DIST)/eqlogui_$(HOST_OS)_$(HOST_ARCH)$(EXT_HOST)" -C "$(EQLOGUI_DIR)" .

# --------------------------------------
# Cross-build eqlog + eqloghub (no CGO)
# --------------------------------------

GOX_ENVS := \
	linux/amd64 linux/arm64 \
	darwin/amd64 darwin/arm64 \
	windows/amd64 windows/arm64

build-all: build-eqlog-all build-eqloghub-all

build-eqlog-all: $(DIST)
	@set -e; \
	for t in $(GOX_ENVS); do \
		os=$$(echo "$$t" | cut -d/ -f1); arch=$$(echo "$$t" | cut -d/ -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "==> eqlog $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -o "$(DIST)/eqlog_$${os}_$${arch}$${ext}" $(EQLOG_PKG); \
	done

build-eqloghub-all: $(DIST)
	@set -e; \
	for t in $(GOX_ENVS); do \
		os=$$(echo "$$t" | cut -d/ -f1); arch=$$(echo "$$t" | cut -d/ -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "==> eqloghub $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -o "$(DIST)/eqloghub_$${os}_$${arch}$${ext}" $(EQLOGHUB_PKG); \
	done

# --------------------------------------
# Convenience: DigitalOcean (linux/amd64)
# --------------------------------------

build-linux-amd64: build-eqlog-linux-amd64 build-eqloghub-linux-amd64

build-eqlog-linux-amd64: $(DIST)
	@echo "==> eqlog linux/amd64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o "$(DIST)/eqlog_linux_amd64" $(EQLOG_PKG)

build-eqloghub-linux-amd64: $(DIST)
	@echo "==> eqloghub linux/amd64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o "$(DIST)/eqloghub_linux_amd64" $(EQLOGHUB_PKG)

# --------------------------------------
# Wails builds (eqlogui)
# --------------------------------------
# Note: Cross-building Wails generally requires OS-specific toolchains.
# These targets are best-effort and will fail if toolchains are missing.

build-eqlogui-wails:
	@command -v $(WAILS) >/dev/null 2>&1 || { echo "wails not found; install wails to use this target"; exit 1; }
	@echo "==> eqlogui (wails) $(HOST_OS)/$(HOST_ARCH)"
	$(WAILS) build -clean -o eqlogui -C "$(EQLOGUI_DIR)"
	mkdir -p "$(DIST)"
	@if [ -f "$(EQLOGUI_DIR)/build/bin/eqlogui" ]; then cp "$(EQLOGUI_DIR)/build/bin/eqlogui" "$(DIST)/eqlogui_$(HOST_OS)_$(HOST_ARCH)"; fi
	@if [ -f "$(EQLOGUI_DIR)/build/bin/eqlogui.exe" ]; then cp "$(EQLOGUI_DIR)/build/bin/eqlogui.exe" "$(DIST)/eqlogui_$(HOST_OS)_$(HOST_ARCH).exe"; fi

build-eqlogui-wails-all:
	@command -v $(WAILS) >/dev/null 2>&1 || { echo "wails not found; install wails to use this target"; exit 1; }
	mkdir -p "$(DIST)"
	@echo "==> eqlogui (wails) windows/amd64"
	$(WAILS) build -clean -platform "windows/amd64" -C "$(EQLOGUI_DIR)"
	@if [ -f "$(EQLOGUI_DIR)/build/bin/eqlogui.exe" ]; then cp "$(EQLOGUI_DIR)/build/bin/eqlogui.exe" "$(DIST)/eqlogui_windows_amd64.exe"; fi
	@echo "==> eqlogui (wails) linux/amd64"
	$(WAILS) build -clean -platform "linux/amd64" -C "$(EQLOGUI_DIR)"
	@if [ -f "$(EQLOGUI_DIR)/build/bin/eqlogui" ]; then cp "$(EQLOGUI_DIR)/build/bin/eqlogui" "$(DIST)/eqlogui_linux_amd64"; fi
	@echo "==> eqlogui (wails) darwin/amd64"
	$(WAILS) build -clean -platform "darwin/amd64" -C "$(EQLOGUI_DIR)"
	@if [ -f "$(EQLOGUI_DIR)/build/bin/eqlogui" ]; then cp "$(EQLOGUI_DIR)/build/bin/eqlogui" "$(DIST)/eqlogui_darwin_amd64"; fi
