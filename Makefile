# Mellowtel Consumer — build & test targets.
#
# Requires: Go (1.23+; the toolchain auto-fetches go1.26 via GOTOOLCHAIN=auto),
# Node.js 18+, and the Wails v2 CLI. On Linux, GUI builds also need
# libgtk-3-dev, libwebkit2gtk-4.0-dev (or 4.1) and pkg-config — see README.

APP        := mellowtel-consumer
WAILS      := wails
DIST       := dist
export GOTOOLCHAIN ?= auto

# Ubuntu 24.04+ ships WebKit 4.1; Wails defaults to 4.0, so pass the 4.1 tag.
# Override with `make WEBKIT_TAGS= ...` on distros that still have 4.0.
WEBKIT_TAGS ?= webkit2_41

.PHONY: all deps test vet build-linux build-windows build-mac dev clean tools

all: build-linux

## Install the Wails CLI and JS deps.
tools:
	go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1

deps:
	cd frontend && npm install

## Run Go unit tests (no GUI/webkit needed).
test:
	go test ./internal/...

vet:
	go vet ./internal/...

## Live-reload development (needs webkit dev libs on Linux).
dev:
	$(WAILS) dev -tags "$(WEBKIT_TAGS)"

## Same as dev, but scrubs snap-injected env vars first.
## Use this if launching from the VS Code *snap* integrated terminal.
dev-clean:
	./scripts/dev.sh

## Linux build -> dist/linux/
build-linux:
	$(WAILS) build -tags "$(WEBKIT_TAGS)" -platform linux/amd64 -o $(APP)
	@mkdir -p $(DIST)/linux
	@cp build/bin/$(APP) $(DIST)/linux/
	@echo "Built $(DIST)/linux/$(APP)"

## Windows .exe -> dist/windows/
## -skipbindings is REQUIRED when cross-compiling from Linux/macOS: Wails
## generates bindings by executing a host binary, which can't be a Windows .exe.
## It reuses the committed bindings in frontend/wailsjs, so run `wails dev` (or a
## native build) once after changing any bound Go method on App.
build-windows:
	$(WAILS) build -platform windows/amd64 -skipbindings -o $(APP).exe
	@mkdir -p $(DIST)/windows
	@cp build/bin/$(APP).exe $(DIST)/windows/
	@echo "Built $(DIST)/windows/$(APP).exe"

## Windows NSIS installer -> dist/windows/  (needs: apt install nsis)
build-windows-installer:
	$(WAILS) build -platform windows/amd64 -skipbindings -nsis -o $(APP).exe
	@mkdir -p $(DIST)/windows
	@cp build/bin/*installer.exe $(DIST)/windows/ 2>/dev/null || true
	@echo "Installer in $(DIST)/windows/"

## macOS .app -> dist/mac/   MUST be run ON a Mac (Xcode CLT required).
## Cross-compiling macOS from Linux is not supported by Wails.
build-mac:
	$(WAILS) build -platform darwin/universal
	@mkdir -p $(DIST)/mac
	@cp -R build/bin/*.app $(DIST)/mac/ 2>/dev/null || true
	@echo "Built macOS app bundle in $(DIST)/mac/"

## macOS .dmg -> dist/mac/    Run ON a Mac, after build-mac.
## Uses create-dmg if present (brew install create-dmg), else falls back to hdiutil.
build-mac-dmg: build-mac
	@if command -v create-dmg >/dev/null 2>&1; then \
		create-dmg --volname "Mellowtel" --window-size 600 400 \
			--icon-size 100 --app-drop-link 400 190 \
			"$(DIST)/mac/Mellowtel.dmg" "$(DIST)/mac/" || true; \
	else \
		hdiutil create -volname "Mellowtel" -srcfolder "$(DIST)/mac" \
			-ov -format UDZO "$(DIST)/mac/Mellowtel.dmg"; \
	fi
	@echo "DMG at $(DIST)/mac/Mellowtel.dmg"

clean:
	rm -rf $(DIST) build/bin
	rm -f $(APP) $(APP).exe
