# Testing Guide

How to verify each milestone. Automated tests run with `make test`
(`go test ./internal/...`) and need no GUI libraries. Manual GUI testing needs
the Wails dev libs (see the README's Linux section) and runs via `wails dev`.

---

## Proving the app actually scrapes

Two ways — one automated, one you can watch live. Both use a page whose real
content is injected by **JavaScript**, so a browser render and a plain HTTP
fetch produce provably different output. (The marker string is assembled at
runtime from two halves, so it never appears in the raw HTML source — otherwise
a plain fetch would "find" it in the script tag and prove nothing.)

### 1. Automated end-to-end test (fastest)

```bash
MELLOWTEL_LIVE=1 go test ./internal/node/ -run TestEndToEndScrape -v
```

Spins up local stand-ins for the registration socket, approval endpoint and
result sink, runs the **real** Manager (real Chrome, real job pipeline), and
asserts:

- the **browser job** contains the JS-injected marker and the placeholder is gone
  → a real browser executed the page's JavaScript;
- the **fetch job** contains the untouched placeholder and no marker
  → the simple path really is plain HTTP;
- both results carry the correct `node_identifier`, `statusCode` 200 and markdown;
- **shutdown completes promptly** (regression guard: a blocking WebSocket read
  once made Disconnect take 68s).

Expected: `connect ~0.5s`, `both results ~2.6s`, `disconnect ~25ms`.

### 2. Watch it live with the local test server

```bash
go run ./testserver          # listens on :8899, prints instructions
```

Point `~/.config/Mellowtel/config.json` at it:

```json
"endpoints": {
  "websocketURL":          "ws://localhost:8899",
  "approvalURL":           "http://localhost:8899/approval",
  "defaultResultEndpoint": "http://localhost:8899/result"
}
```

Restart the app (`make dev-clean`) and press **Connect**. The server prints the
node's device_id on connect, sends two jobs, then prints each result with a
verdict and writes the full scraped payloads to a temp dir for inspection.

Restore the original endpoints in `config.json` when done.

---

## Automated tests

```bash
make test
```

Covers: job routing decision, wire-quirk job parsing (`internal/job`), markdown
conversion (`internal/markdown`), device-ID format/persistence/rewrite
(`internal/device`), pause-schedule + bandwidth cap (`internal/node`), CDP header
filtering (`internal/browser`), WebSocket registration URL + message dispatch
against a live local socket (`internal/wsclient`), and an end-to-end job through
the executor with a stub renderer and an `httptest` result sink
(`internal/executor`).

---

## Milestone 1 — Skeleton

1. `wails dev` opens a ~420×640 window titled **Mellowtel**.
2. The header shows the brand + a **⚙** settings button.
3. A large circular **Connect** button is centered; it is grey when paused.
4. A system-tray icon appears (grey). Its menu has **Show / Connect / Quit**.
5. The device ID at the bottom is populated and is copyable (click it).
6. Confirm the config dir was created and holds `config.json`, `device_id`,
   `mellowtel.log` (Linux: `~/.config/Mellowtel/`).

**Expected:** clicking Connect flips the button/tray toward the connected state;
`mellowtel.log` gains structured lines for each action.

---

## Milestone 2 — WebSocket connection

Point the app at a local stub to avoid production. In `config.json` set
`endpoints.websocketURL` to `ws://localhost:8080` and
`endpoints.approvalURL` to `http://localhost:8081/approval`.

Minimal echo/heartbeat server (Go):

```go
// go run ./testserver.go  — upgrades WS, sends a heartbeat then stays open.
```

1. Click **Connect** → status shows **Connecting…** then **Connected** (green).
2. Kill the stub server → status shows **Reconnecting…**; restart it → it
   reconnects (exponential backoff visible in the log).
3. Watch the log for a WS ping every ~60 s.
4. Approval: have the stub return `{"approval": true}` → node keeps running;
   return `{"approval": false}` → status shows **Approval failed** and the node
   pauses.

The `internal/wsclient` unit test already exercises the connect + dispatch +
heartbeat-filter path automatically.

---

## Milestone 3 — Simple job execution

With the WS stub connected, push a `fetchInstead` job frame and a local result
sink:

```jsonc
// Send over the WS as a text frame:
{
  "recordID": "test-1",
  "url": "https://example.com",
  "fetchInstead": true,
  "saveMarkdown": true,
  "saveHtml": true,
  "save_html_endpoint": "http://localhost:8081/result"
}
```

**Expected:** the log shows `job queued` → `executing job` (path=simple) →
`job completed`. Your result sink receives a POST (`Content-Type: text/plain`)
whose JSON body contains `recordID`, `node_identifier` (the device ID),
`final_url`, `statusCode`, and a non-empty `markDown` (and `content` since
`saveHtml` was set). The **jobs done** counter in the UI increments.

The `internal/executor` unit test asserts this exact payload shape.

---

## Milestone 4 — Browser job execution

Requires Chrome/Chromium installed.

1. Rename/remove Chrome (or run on a machine without it) → on launch a **Chrome
   required** modal appears with a **Download Chrome** button; the UI shows a
   "Chrome not found" pill. Simple (`fetchInstead`) jobs still run.
2. With Chrome present, push a rendering job (omit `fetchInstead`):

```jsonc
{
  "recordID": "test-2",
  "url": "https://news.ycombinator.com",
  "saveMarkdown": true,
  "waitBeforeScraping": 2,
  "actions": "[{\"type\":\"scroll\",\"direction\":\"down\",\"amount\":1000},{\"type\":\"wait\",\"milliseconds\":500}]",
  "save_html_endpoint": "http://localhost:8081/result"
}
```

**Expected:** headless Chrome launches against the isolated
`chrome-profile/` dir (your real profile is untouched); the log shows path=browser;
the result POST contains rendered HTML/markdown. To confirm header stripping,
target a site that sends `X-Frame-Options`/CSP — it should still render.

---

## Milestone 5 — Settings & polish

1. **⚙ Settings** opens the panel. Toggle **Auto-connect** and relaunch → the app
   connects on startup.
2. **Launch on system startup** → verify the OS entry
   (Linux: `~/.config/autostart/mellowtel.desktop`; macOS:
   `~/Library/LaunchAgents/tel.mellow.consumer.plist`; Windows: `HKCU\...\Run`).
3. **Close to tray** on → clicking the window's **✕** hides it (app keeps running,
   tray stays). Use the tray **Show** to restore, **Quit** to exit fully.
4. **Bandwidth cap** = 1 GB/day → after simulated usage crosses the cap, new jobs
   are skipped and the status shows **Bandwidth cap reached** (unit-tested in
   `internal/node`).
5. **Pause schedule** → set a window covering "now"; incoming jobs are skipped and
   the status shows **Paused (schedule)** (unit-tested).
6. **View logs** opens the config folder.

---

## Cross-platform

- **Windows:** run in a VM. Verify tray icon + menu, Chrome detection (registry +
  common paths), WebSocket connect, and one full job cycle.
- **macOS:** MVP target is a GitHub Actions build (`.github/workflows/build.yml`);
  real-device testing before v1. Note the tray needs the main thread on macOS.
