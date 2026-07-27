# Mellowtel Consumer

A cross-platform **consumer desktop app** for Mellowtel — bandwidth-sharing in
the style of Grass.io / Honeygain / IdleForest. It runs on the user's machine,
connects to Mellowtel's existing infrastructure over WebSocket, receives scrape
jobs, executes them using the user's **already-installed Chrome** (via CDP), and
returns the results.

- **Language:** Go
- **UI:** [Wails v2](https://wails.io) (Go backend + React/TypeScript frontend)
- **Browser control:** [chromedp](https://github.com/chromedp/chromedp) (drives installed Chrome over CDP)
- **WebSocket:** gorilla/websocket · **Logging:** zerolog · **Markdown:** html-to-markdown

> No browser is bundled. The app uses the user's installed Chrome/Chromium and
> prompts to install Chrome if none is found.

---

## Status (MVP)

All five MVP milestones are implemented:

| Milestone | Scope | State |
|---|---|---|
| M1 | Scaffold, config dir, persistent device ID, logging, tray, Connect button | ✅ |
| M2 | WebSocket to `ws.mellow.tel`, heartbeat, backoff reconnect, approval polling | ✅ |
| M3 | Job router, simple HTTP-fetch path, Markdown, result POST | ✅ |
| M4 | Chrome detection, chromedp render, CDP `Fetch` header stripping, actions, extract | ✅ |
| M5 | Settings screen, auto-launch, close-to-tray, bandwidth cap, pause schedule | ✅ |

Go unit tests cover the job router, request parsing, markdown conversion, device
ID generation, the pause-schedule and bandwidth logic, and an end-to-end
executor path. Run `make test`.

---

## Prerequisites

- **Go 1.23+** — the module targets `go 1.26` (a chromedp transitive dep); with
  the default `GOTOOLCHAIN=auto`, Go automatically downloads the 1.26 toolchain
  on first build. Nothing to do manually.
- **Node.js 18+** and npm (for the frontend).
- **Wails v2 CLI:** `go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1`
- **Google Chrome or Chromium** installed (runtime requirement for browser jobs).

### Linux GUI system libraries (required for `wails dev` / `wails build`)

Building the actual desktop GUI on Linux needs native WebKit/GTK dev packages.
The pure-Go backend and the unit tests do **not** need these, but the windowed
app does:

```bash
sudo apt update
sudo apt install -y libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config
# On newer distros where 4.0 is unavailable, use:  libwebkit2gtk-4.1-dev
# Optional (Windows installer generation): sudo apt install -y nsis
```

Verify your toolchain any time with `wails doctor`.

> **Ubuntu 24.04+ / WebKit 4.1.** These distros ship `libwebkit2gtk-4.1-dev`, but
> Wails defaults to compiling against 4.0. Build/run with the 4.1 tag:
> `wails dev -tags webkit2_41` (the `make dev` / `make build-linux` targets already
> pass it). Without the tag the native build fails and `wails dev` silently falls
> back to a browser dev server on `http://localhost:34115` — no desktop window.

> **VS Code *snap* terminal crash.** If the built app crashes at startup with
> `symbol lookup error: /snap/core20/.../libpthread.so.0: undefined symbol
> __libc_pthread_init`, you launched it from the VS Code **snap**'s integrated
> terminal, which injects `GTK_PATH`/`SNAP_*` vars that redirect GTK to snap's
> old glibc. Fix: run from a normal system terminal (`Ctrl+Alt+T`), or use
> `make dev-clean` / `./scripts/dev.sh`, which scrub those variables first.

> These packages require root. If you cannot install them, you can still run all
> unit tests and compile the backend (`make test`, `go build ./internal/...`);
> only the GUI (`wails dev` / `wails build`) needs them.

---

## Quick start (development)

```bash
# 1. Install the Wails CLI (once)
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1

# 2. Install frontend deps
cd frontend && npm install && cd ..

# 3. Run with live reload (regenerates Go<->JS bindings automatically)
wails dev
```

`wails dev` opens the window and hot-reloads the frontend. It also (re)generates
the typed bindings in `frontend/wailsjs/` from the current Go `App` methods.

---

## Building

Outputs land in `dist/<platform>/`.

```bash
make build-linux              # -> dist/linux/mellowtel-consumer
make build-windows            # -> dist/windows/mellowtel-consumer.exe
make build-windows-installer  # -> dist/windows/*installer.exe   (needs nsis)
make build-mac                # -> dist/mac/Mellowtel.app        (RUN ON A MAC)
make build-mac-dmg            # -> dist/mac/Mellowtel.dmg        (RUN ON A MAC)
make test                     # Go unit tests (no GUI libs needed)
```

or `./build.sh linux|windows|mac|test|all`.

### What can and cannot be cross-compiled (verified)

Wails generates JS bindings by **compiling and executing a host binary**, so
cross-compiling fails unless that step is skipped.

| Target | From Linux | Notes |
|---|---|---|
| Linux | ✅ native | needs `-tags webkit2_41` on Ubuntu 24.04+ |
| **Windows** | ✅ **yes, with `-skipbindings`** | verified: produces a real `PE32+ (GUI) x86-64` exe |
| **macOS** | ❌ **no** | needs Xcode CLT + darwin SDK; build on a Mac |

Without `-skipbindings` a Windows cross-build dies with
`fork/exec /tmp/wailsbindings: exec format error`. The Makefile passes it for
you. Because it reuses the committed bindings in `frontend/wailsjs/`, run
`wails dev` (or one native build) after changing any bound method on `App` so
those bindings are regenerated first.

---

## Building for macOS

macOS builds **must run on a Mac**. On your Mac:

```bash
# 1. Prerequisites
xcode-select --install                 # Xcode Command Line Tools (required)
brew install go node                   # or install Go/Node manually
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
export PATH="$HOME/go/bin:$PATH"

# 2. Get the code + frontend deps
cd mellowtel-consumer/frontend && npm install && cd ..

# 3. Run it live
wails dev

# 4. Build the app bundle (Intel + Apple Silicon universal)
make build-mac                         # -> dist/mac/Mellowtel.app
open dist/mac/Mellowtel.app            # or double-click it

# 5. Optional: wrap it in a .dmg for distribution
brew install create-dmg                # optional but nicer output
make build-mac-dmg                     # -> dist/mac/Mellowtel.dmg
```

`wails build -platform darwin/universal` produces a universal binary. For a
single architecture use `darwin/arm64` (Apple Silicon) or `darwin/amd64` (Intel).

**macOS specifics for this app:**

- **Gatekeeper.** An unsigned `.app` triggers *"cannot be opened because the
  developer cannot be verified."* For personal use: right-click → **Open**, or
  `xattr -cr dist/mac/Mellowtel.app`. To distribute publicly you need an Apple
  Developer ID, `codesign`, and notarisation (`xcrun notarytool`) — out of MVP
  scope.
- **System tray.** macOS requires the tray to own the **main thread**, but Wails
  already owns it. The tray runs on a goroutine here (fine on Linux/Windows), so
  on macOS it may not appear. The window, jobs, and everything else work
  normally. Proper macOS tray support needs restructuring `main.go` to run
  `systray.Run` on the main thread — a known MVP limitation.
- **Chrome** is auto-detected at `/Applications/Google Chrome.app/...`.
- **Launch on startup** installs a LaunchAgent at
  `~/Library/LaunchAgents/tel.mellow.consumer.plist`.

---

## Building for Windows

**Option A — cross-compile from Linux (fastest, verified working):**

```bash
make build-windows            # -> dist/windows/mellowtel-consumer.exe
sudo apt install -y nsis      # optional, for an installer
make build-windows-installer  # -> dist/windows/*installer.exe
```
Copy the `.exe` to a Windows machine/VM and run it — no install required.

**Option B — build natively on Windows:**

```powershell
# Install Go + Node + WebView2 runtime, then:
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
cd frontend; npm install; cd ..
wails build            # -> build\bin\mellowtel-consumer.exe
wails build -nsis      # installer
```

**Windows specifics:** the app needs the **WebView2 runtime** — preinstalled on
Windows 11 and current Windows 10; otherwise Wails prompts to install it (or use
`-webview2 embed` to bundle the bootstrapper). Tray icon and menu work. Launch
on startup uses `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

---

## How it works (architecture)

```
main.go / app.go        Wails entry + bound methods exposed to the frontend
internal/
  config/               Load/save config.json + user settings; all endpoints live here
  device/               Persistent node ID  mllwtl_<integration>_<rand10>
  logging/              zerolog to stderr + rotating log file
  wsclient/             wss://ws.mellow.tel — query-param registration, ping/pong heartbeat, backoff reconnect
  approval/             GET https://api.mellow.tel/approval every 30 min
  job/                  Job schema, wire-quirk parser, router decision (simple vs browser)
  fetch/                Plain net/http path (fetchInstead / custom method endpoint)
  browser/              Chrome detection + chromedp engine + CDP Fetch header stripping + actions
  markdown/             HTML -> Markdown (ATX headings, fenced code, "*" bullets)
  result/               POST result payload to save_html_endpoint (Content-Type: text/plain)
  executor/             Orchestrates one job end-to-end: route -> render/fetch -> convert -> submit
  node/                 Manager: owns connect/disconnect, worker pool, bandwidth cap, pause schedule
  tray/                 System-tray icon + menu (best-effort, non-fatal)
  autostart/            Launch-on-login (per-OS: .desktop / LaunchAgent / Run key)
frontend/               React + TypeScript UI (Grass-style single window)
```

### Protocol summary

- **Registration:** the client connects to `wss://ws.mellow.tel` with the
  device ID, version, platform and download-speed carried as **query
  parameters** (there is no JSON handshake). Heartbeat is WebSocket protocol
  ping every 60 s; app-level `{"type_event":"heartbeat"}` frames are ignored;
  `{"type_event":"disconnect_device"}` pauses the node.
- **Jobs** arrive as JSON. Several fields are string-encoded on the wire
  (`actions` is a JSON string, `screen_width`/`height` look like `"1024px"`,
  booleans may be `"true"`); the parser in `internal/job` normalises all of them.
- **Routing:** `fetchInstead` or a custom `method_endpoint` → plain HTTP fetch;
  otherwise the page is rendered in headless Chrome so client-side JS runs.
- **Header stripping (browser path): implemented but OFF by default.** The CDP
  `Fetch` domain can strip `X-Frame-Options`/`Content-Security-Policy` (and
  COOP/COEP/CORP), but pages here load as **top-level navigations**, where those
  headers don't apply — exactly why the reference Electron SDK doesn't strip them
  either. Interception also pauses every matched response, adding latency. Enable
  with `"stripFramingHeaders": true` in `config.json` only if you render inside a
  frame. Per-job `skipHeaders` still suppresses it.
- **Results** are POSTed to the job's `save_html_endpoint` (default
  `https://request.mellow.tel/`) as a JSON body with `Content-Type: text/plain`.
  `content` (HTML) and `markDown` are included only when requested and are sent
  uncompressed, matching the reference SDK.

> **Integration key / platform label.** The device ID's `<integration>` segment
> defaults to `consumer` and the reported platform prefix to `desktop`
> (`internal/config`). If Mellowtel requires this client to present a specific
> registered integration id or platform string, change `Integration` /
> `PlatformPrefix` in the config (or `config.Defaults()`), or edit the saved
> `config.json` — no code elsewhere hardcodes these.

---

## Configuration & data locations

Everything lives under the OS user-config dir, in a `Mellowtel/` folder:

| OS | Path |
|---|---|
| Linux | `~/.config/Mellowtel/` |
| macOS | `~/Library/Application Support/Mellowtel/` |
| Windows | `%AppData%\Mellowtel\` |

Contents: `config.json` (endpoints + settings), `device_id`, `mellowtel.log`,
and `chrome-profile/` (an isolated Chrome user-data-dir — the user's real
browsing profile is never touched).

Settings (auto-connect, launch-on-startup, close-to-tray, bandwidth cap, pause
schedule, max concurrent jobs) are editable in the in-app **⚙ Settings** panel.

---

## Testing

```bash
make test            # or: go test ./internal/...
go vet ./internal/...
```

See [docs/TESTING.md](docs/TESTING.md) for a per-milestone manual test plan
(including how to point the app at a local WebSocket/result stub to exercise the
full job cycle without hitting production).

---

## What's intentionally **not** in this MVP

Accounts/login, payouts, earnings ledger, referrals, anti-abuse, screenshot job
types (`htmlVisualizer`/`htmlContained`), POST-method scraping, cereal parsing,
and the auto-updater. These are planned for later phases. Scope here is:
**install → connect → execute jobs → return results.**
