# Mellowtel Consumer — Complete Technical Guide

Everything about this app: what it is, what it's built with, how the UI and
backend connect, how it talks to Mellowtel, how it drives the browser, how it's
packaged, and where every piece lives in the code.

Every claim below cites `file:line` so you can open the real code.

---

# 1. What the app is (the 30-second version)

A **cross-platform desktop app that lets a user share their internet connection
and earn from it** — the Grass.io / Honeygain / IdleForest model.

It runs on the user's machine, connects to Mellowtel's servers, receives
**web-scraping jobs**, fetches those pages **using the user's own Chrome and
home internet connection**, and sends the extracted content back.

**Why residential nodes exist:** websites block datacenter/cloud IPs. Mellowtel's
customers need web data, so the traffic is routed through consenting home
connections instead. The user's bandwidth and IP address are the product; they
consent explicitly by pressing Connect.

---

# 2. Technology stack

## Backend — Go 1.26 (~4,400 lines)

| Purpose | Library | Version |
|---|---|---|
| Desktop app framework | `github.com/wailsapp/wails/v2` | v2.10.1 |
| Browser automation (CDP) | `github.com/chromedp/chromedp` + `cdproto` | v0.16.0 |
| WebSocket client | `github.com/gorilla/websocket` | v1.5.3 |
| Structured logging | `github.com/rs/zerolog` | v1.35.1 |
| HTML → Markdown | `github.com/JohannesKaufmann/html-to-markdown` | v1.6.0 |
| System tray | `fyne.io/systray` | v1.12.2 |
| Windows registry (autostart) | `golang.org/x/sys` | v0.47.0 |

Config is plain `encoding/json` — no Viper. HTTP is Go's standard `net/http`.
Concurrency is standard Go: goroutines, channels, `context`, `sync.Mutex`.

## Frontend — React + TypeScript (~500 lines)

React 18.2, TypeScript 4.6, Vite 3 as the bundler. Three components, hand-written
CSS, no UI framework and no state library (Redux/Zustand). Bundle: **47 KB gzipped**.

## The critical architectural point

**Two different browser engines do two different jobs:**

1. **The app's UI** renders in the OS's *native* webview — WebKitGTK (Linux),
   WKWebView (macOS), WebView2 (Windows). Nothing is bundled.
2. **The scraping** drives the user's *installed Google Chrome* as a separate
   headless process over the DevTools Protocol.

This is why the binary is **14 MB**, not ~150 MB. Electron would bundle an entire
Chromium runtime for the UI; Wails borrows the OS's.

> **Meeting answer to "why not Electron?"** — Electron ships Chromium with every
> app (~150 MB) and runs the UI in it. Wails uses the OS's built-in webview, so
> the binary is ~10× smaller. And because we need a *real* Chrome for scraping
> anyway, bundling a second browser for the UI would be pure waste.

---

# 3. How the UI and backend are connected

Wails does three things that bridge Go and JavaScript.

### a) Generated bindings (JS calls Go)

Every exported method on the `App` struct is auto-exposed to JavaScript. Wails
introspects them at build time and generates `frontend/wailsjs/go/main/App.js`.

The bound methods — **`app.go:81-174`**:

| Go method | file:line | What the UI uses it for |
|---|---|---|
| `Connect()` | app.go:81 | Start sharing |
| `Disconnect()` | app.go:87 | Pause sharing |
| `Toggle()` | app.go:93 | The big power button |
| `GetStatus()` | app.go:106 | Poll current state/counters |
| `GetDeviceID()` | app.go:111 | Show the node ID |
| `GetVersion()` | app.go:116 | Footer version |
| `IsChromeInstalled()` | app.go:121 | Decide whether to warn |
| `ChromeDownloadURL()` | app.go:126 | "Download Chrome" button |
| `GetSettings()` | app.go:131 | Populate settings form |
| `SaveSettings()` | app.go:136 | Persist settings |
| `OpenLogsFolder()` | app.go:159 | "View logs" |
| `GetLogPath()` | app.go:164 | Show log path |
| `OpenURL()` | app.go:169 | Open dashboard link |
| `ShowWindow()` | app.go:174 | Tray → Show |

From TypeScript these are just async functions. The frontend wraps them in a
typed layer at `frontend/src/api.ts`, which calls `window.go.main.App.*` and
returns Promises. Wails marshals arguments and return values as JSON over an
internal bridge.

### b) Event bus (Go pushes to JS)

The backend emits events the UI subscribes to:

- Go side: `wailsruntime.EventsEmit(a.ctx, "status:update", s)` — **app.go:40**
- JS side: `subscribe('status:update', ...)` in `frontend/src/api.ts`

This is wired in `App.startup` (**app.go:35-50**): the manager gets a callback
that emits to the frontend *and* updates the tray icon.

> **Known issue:** the event subscription doesn't always attach, so the UI is
> currently kept fresh by a **2-second polling fallback** (`App.tsx` useEffect)
> that calls `GetStatus()`. Functionally correct, cosmetically imperfect.

### c) Asset embedding

Vite builds the React app into `frontend/dist`, and `//go:embed all:frontend/dist`
(**main.go:23-24**) bakes those files into the Go binary. The result is a single
self-contained executable — no external files to ship.

**Window configuration** lives in `main.go:54-79`: 420×640, dark background,
plus lifecycle hooks `OnStartup` / `OnShutdown` / `OnBeforeClose`.

---

# 4. Mellowtel integration — every API, where it's called

There are **exactly three** Mellowtel endpoints. All are defined in one place,
`internal/config/config.go` (`Defaults()`), so nothing is hardcoded elsewhere:

```go
WebSocketURL:          "wss://ws.mellow.tel"
ApprovalURL:           "https://api.mellow.tel/approval"
DefaultResultEndpoint: "https://request.mellow.tel/"
```

## 4.1 The registration WebSocket — `wss://ws.mellow.tel`

**Purpose:** this is how the node joins the network and receives work. It is a
*persistent* connection, not a request/response API.

**Where:** `internal/wsclient/wsclient.go`

**Crucially, there is NO JSON handshake.** All registration data is sent as
**query parameters on the connection URL** — built in `connURL()` at
**wsclient.go:74-88**:

```
wss://ws.mellow.tel?device_id=mllwtl_consumer_5apf20hmt2
                   &version=700.0.29
                   &platform=desktop-linux
                   &speed_download=500
```

| Param | Set at | Meaning |
|---|---|---|
| `device_id` | wsclient.go:80 | Persistent node identity (see §5) |
| `version` | wsclient.go:81 | Protocol version |
| `platform` | wsclient.go:82 | `<prefix>-<os>`, OS resolved by `osLabel()` :62 |
| `speed_download` | wsclient.go:84 | Mbps, currently hardcoded 500 (manager.go:182) |

The actual dial: **wsclient.go:140** (`websocket.DefaultDialer.DialContext`).

**Direction of traffic:** after connecting, the client **only listens**. It never
sends job data upstream on this socket — results go over HTTPS instead (§4.3).

### Heartbeat / keep-alive

- Client sends a **WebSocket protocol ping every 60s** — `pinger()`
  **wsclient.go:182-190**.
- A pong resets the read deadline — **wsclient.go:150-152** (`pongWait` = 70s).
- If the server goes silent, the read deadline trips, the connection drops, and
  the reconnect loop takes over.

### Inbound message handling — `handleMessage()` wsclient.go:199

| Message | Handling | Line |
|---|---|---|
| `{"type_event":"heartbeat"}` | Ignored by design | :203 |
| `{"type_event":"disconnect_device"}` | Server orders node to pause | :206 |
| Anything else with a `url` + `recordID` | Treated as a **scrape job** | passed to `OnMessage` |

### Reconnection

`Run()` (**wsclient.go:98**) loops forever until the context is cancelled, with
exponential backoff 5s → 60s (`internal/wsclient/helpers.go`). The UI shows
`Connecting…` / `Reconnecting…` via the `OnState` callback (manager.go:360).

## 4.2 The approval API — `GET https://api.mellow.tel/approval`

**Purpose:** a kill-switch/authorisation gate. Mellowtel can revoke a node.

**Where:** `internal/approval/approval.go`

- Request built and sent: **approval.go:48-70**, method `GET` at **:65**
- Query params (`device_id`, `version`, `platform`, `speed_download`): **:54-58**
- **Response parsed at :81** — the only field read is `approval`:
  ```json
  { "approval": true }
  ```
- **Cadence:** immediately on connect, then **every 30 minutes** — `CheckInterval`
  **approval.go:18**, ticker at **:105**.

**Callback handling — `onApproval()` manager.go:380:**

| Outcome | Behaviour |
|---|---|
| `approval: true` | Keep running |
| `approval: false` | Status → "Approval failed", node disconnects itself |
| Network error | **Deliberately ignored** — a flaky connection must not kill a healthy node |

## 4.3 The result API — `POST https://request.mellow.tel/`

**Purpose:** where scraped content is delivered. This is how work is attributed
to the node (and ultimately how earnings would be credited).

**Where:** `internal/result/result.go`

- The POST: **result.go:69**
- **`Content-Type: text/plain`** — **result.go:74**. Note: the body *is* JSON,
  but the SDK sends it as `text/plain`, and we match that exactly for compatibility.
- Endpoint is **per-job**: each job carries `save_html_endpoint`; the config value
  is only the fallback (executor.go:146-148).

**The exact payload** — struct at **result.go:20-40**:

```jsonc
{
  "recordID":            "...",   // the customer's job ID
  "node_identifier":     "mllwtl_consumer_5apf20hmt2",  // ATTRIBUTION KEY
  "url":                 "...",
  "final_url":           "...",   // after redirects
  "statusCode":          200,
  "website_unreachable": false,
  "fastLane":            true,
  "orgId":               "...",
  "htmlTransformer":     "none",
  "saveHtml":            true,
  "saveMarkdown":        true,
  "saveText":            false,
  "BATCH_execution":     false,
  "batch_id":            "",
  "cereal_result":       "",
  "file_name_bytes":     "",
  "requestMessageInfo":  { /* the original job echoed back */ },

  "content":  "<html>…</html>",   // only if saveHtml  — uncompressed
  "markDown": "# Page…"           // only if saveMarkdown — uncompressed
}
```

`content`/`markDown` are pointers (**result.go:39-40**) so they're omitted
entirely when not requested.

**Failed scrapes still POST**, with `website_unreachable: true` — reporting
failure is part of the contract (executor.go:166).

**Timeout isolation (important detail):** submission runs on a context *detached*
from the job deadline — `context.WithoutCancel(ctx)` at **executor.go:194** — with
its own 30s budget (`submitTimeout`, result.go). Without this, a slow page
consumed the job budget and the POST was starved, losing already-scraped data.

---

# 5. Node identity — how Mellowtel knows who did the work

**Where:** `internal/device/device.go`

Format: **`mllwtl_<integration>_<rand10>`** — e.g. `mllwtl_consumer_5apf20hmt2`.

- `mllwtl` — fixed prefix
- `consumer` — the integration key (configurable; identifies this client type)
- `5apf20hmt2` — 10 random base-36 chars, from `crypto/rand`

Persisted to `~/.config/Mellowtel/device_id` and **stable across restarts**. If
the integration key ever changes, the random suffix is preserved and only the key
segment rewritten (matching SDK behaviour).

This string is sent on the WebSocket URL (`device_id`), on approval checks, and
stamped on every result as `node_identifier`. **It is the single thread linking a
scrape back to this machine.**

---

# 6. Browser handling — the deep dive

This is the part most likely to be questioned. Details in
`internal/browser/browser.go`.

## 6.1 Which browser? The user's own — not a bundled one

We **never bundle or download a browser**. On startup, `FindChrome()`
(`internal/browser/detect.go`) locates an existing installation:

- **Linux:** `/usr/bin/google-chrome`, `google-chrome-stable`, `chromium`,
  `chromium-browser`, `/snap/bin/chromium`, plus a `$PATH` search
- **macOS:** `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
- **Windows:** `%ProgramFiles%`, `%ProgramFiles(x86)%`, `%LocalAppData%` under
  `Google\Chrome\Application\chrome.exe`

If nothing is found, the UI shows a "Chrome required" modal with a download link,
and jobs fall back to plain HTTP fetching.

## 6.2 A separate, isolated instance — the user's browsing is untouched

We launch a **completely separate Chrome process** with its own profile
directory:

```
--user-data-dir = ~/.config/Mellowtel/chrome-profile/
```
Set at **manager.go:151** and **browser.go:117**.

**We deliberately do NOT attach to a Chrome the user is already running.** Why
this matters:

- Their **cookies, logins, history, and sessions are never touched or read**
- Scraping can't hijack their authenticated sessions
- Their normal browsing isn't disturbed

**Launch flags — browser.go:116-124:**

| Flag | Purpose |
|---|---|
| `--headless=new` | Modern headless mode — no visible window |
| `--disable-gpu` | Not needed headless |
| `--no-first-run`, `--no-default-browser-check` | Suppress setup prompts |
| `--disable-background-networking` | No telemetry/update chatter |
| `--disable-extensions` | Clean, predictable page loads |
| `--mute-audio` | Silence any autoplay |

## 6.3 Cookies and session state

Cookies live in that isolated profile directory and **persist on disk across
runs**, shared between jobs (all jobs run as tabs in one browser, so one cookie
jar). Practical effect: a site that sets a session cookie on job 1 will see it on
job 2. It is completely separate from the user's real Chrome profile.

To wipe all scraping state: delete `~/.config/Mellowtel/chrome-profile/`.

## 6.4 One browser, many tabs — and the bug this fixed

**`Start()` — browser.go:95-146** launches **exactly one** Chrome process for the
whole session:

- `chromedp.NewExecAllocator(...)` — **browser.go:127** — defines *how* to launch
- `chromedp.NewContext(allocCtx)` + `Run` — **browser.go:131-132** — actually
  starts the browser

Then **each job opens a new TAB in that same browser** —
`chromedp.NewContext(parent)` at **browser.go:190**, where `parent` is the
*browser* context.

> **Why this matters (real bug we hit):** originally each job created a context
> from the *allocator*, which makes chromedp launch a **brand-new Chrome per
> job**. All of them pointed at the same profile dir, and Chrome enforces one
> process per profile via `SingletonLock` — so every job after the first died
> with `Failed to create a ProcessSingleton`. One browser + many tabs is both
> correct and far cheaper.

Stale locks from a crashed run are cleared on startup — `clearSingletonLocks()`
**browser.go:168**.

## 6.5 The CDP connection — how it's established and maintained

**CDP = Chrome DevTools Protocol**, the same JSON-over-WebSocket protocol
DevTools itself uses. `chromedp` speaks it for us.

1. Chrome is launched with a debugging port; chromedp connects over a local
   WebSocket. This is **loopback only** — not exposed to the network.
2. Each tab is a **CDP "target"** with its own session. Commands
   (`Page.navigate`, `Runtime.evaluate`, `DOM.getOuterHTML`) are addressed to a
   session; events stream back on the same connection.
3. **Session lifetime is tied to a Go `context`.** This caused our second major
   bug — see below.

**The session-binding rule (browser.go:195-205):**

> chromedp binds a target to whichever context **first runs against it**. We
> therefore materialise the tab session against `tabCtx` up front
> (`chromedp.Run(tabCtx)` at **browser.go:203**) *before* deriving the shorter
> navigation context.

Originally the first `Run` happened under a short-lived `navCtx` that we then
cancelled — which **killed the tab**, so every later action hung until the outer
45s timeout. That's why *every* job was failing with `context deadline exceeded`,
even `example.com`. After the fix, the same pages render in 1–4s.

**Teardown:** the tab context is cancelled after each job (`defer cancelTab()`,
**browser.go:191**), closing the tab. The browser stays alive for reuse and is
only stopped on Disconnect — `Stop()` **browser.go:148**.

## 6.6 How a page is actually scraped

`Render()` — **browser.go:178-337**:

1. **New tab** from the browser context — :190
2. **Bind the session** to the tab context — :203
3. **Register an event listener** for CDP events — :212
4. *(Optional, off by default)* enable `Fetch` interception — :266
5. **Navigate** — `chromedp.Navigate(req.URL)` :274, run under a **25s budget**
   (`navTO`, :81)
6. **On navigation timeout → salvage.** Many pages never finish loading (ad
   trackers), but the DOM is already usable — we keep going instead of failing
7. **Wait** `waitBeforeScraping` seconds (from the job)
8. **Wait for a selector** if the job specifies `waitForElement`
9. **Run the job's actions** in order (`internal/browser/actions.go`): `wait`,
   `click`, `write`, `press`, `fill_input`, `fill_textarea`, `select`,
   `fill_form`, `scroll`, `waitFor`. Missing selectors are non-fatal — one bad
   selector shouldn't sink a good scrape
10. **Extract** `document.documentElement.outerHTML` — `chromedp.OuterHTML` :311
11. **Capture the final URL** (post-redirect)
12. **Close the tab**

Budgets: **25s navigation**, **45s whole render** (browser.go:80-81).

## 6.7 Header stripping (X-Frame-Options / CSP) — built but OFF

The CDP `Fetch` domain can strip `X-Frame-Options`, `Content-Security-Policy`,
COOP/COEP/CORP (`filterHeaders`, **browser.go:341**). It is **disabled by
default** (`stripFramingHeaders` in config).

**Why off:**

1. We load pages as **top-level navigations**, where those headers simply don't
   apply. They only matter when rendering inside an **iframe** — which is what
   Mellowtel's *browser-extension* SDK does. The Electron SDK (our real
   reference) doesn't strip headers either.
2. Interception **pauses every matched response** pending an explicit continue.
   Any failed continue hangs the page forever — we proved this: with it on,
   `example.com` timed out at 20s.

## 6.8 JavaScript challenges and anti-bot — an honest limitation

**What we handle well:** ordinary client-side JavaScript. Because it's a real
Chrome, JS executes, SPAs hydrate, and DOM built at runtime is captured. Our
end-to-end test proves this — a page whose content is injected by JS is captured
by the browser path and *not* by the fetch path.

**What we do NOT handle:** active anti-bot systems (Cloudflare Turnstile,
DataDome, PerimeterX) and CAPTCHAs. We ship **no stealth patches**. The reference
Electron SDK injects a large stealth payload — patching `navigator.webdriver`,
`window.chrome`, plugins, WebGL vendor strings, fake mouse movement — and we
deliberately did not port that for the MVP.

In practice: `--headless=new` is detectable, so hard-protected sites will fail
(you'll see these as `jobsFailed`). Two things do help: it's a **real** Chrome
(not a scripted HTTP client) and the traffic comes from a **residential IP**.

> **If asked in the meeting:** "JS rendering works — that's the whole point of
> using Chrome. Anti-bot evasion is not implemented; that's a known gap, and the
> reference SDK's stealth-injection layer is the roadmap item."

---

# 7. Full job lifecycle — one trace

```
Mellowtel dispatcher
        │  JSON job over the WebSocket
        ▼
wsclient.handleMessage         wsclient.go:199   filter control frames
        ▼
Manager.onMessage              manager.go:276
        ├─ job.Parse           job/request.go    normalise wire quirks
        ├─ shouldSkip          manager.go:306    pause schedule + bandwidth cap
        └─ enqueue (24 slots)  manager.go:141    full → drop
        ▼
worker (×3 default)            manager.go:321
        ▼
Manager.runJob                 manager.go:339    90s job budget
        ▼
Executor.Execute               executor.go:70
        ├─ job.Decide          executor.go:71    simple vs browser
        │     ├─ simple  → fetch.Client.Do      executor.go:136
        │     └─ browser → Engine.Render        executor.go:106
        ├─ markdown.Convert                     executor.go:181
        └─ buildAndSubmit                       executor.go:145
              └─ result.Submit → POST           result.go:69
        ▼
counters + log                 manager.go:339-358
        ▼
UI (event + 2s poll)           app.go:40 / api.ts
```

## The wire-format quirks (a real gotcha)

Jobs arrive loosely typed, so `internal/job/request.go` normalises:

- **`actions` is a JSON-encoded *string***, not an array — it must be parsed twice
- **`screen_width`/`screen_height`** look like `"1024px"` — stripped and parsed
- **Booleans may be strings** — `"true"` as well as `true`
- Defaults match the SDK: `saveMarkdown` true, `waitBeforeScraping` 1,
  1024×768, `fastLane` true

## Routing rule — `job.Decide()`, `internal/job/decision.go`

| Condition | Path |
|---|---|
| `fetchInstead: true` | **Simple** — plain `net/http`, no browser |
| `method_endpoint` set | **Simple** |
| otherwise | **Browser** — headless Chrome, so JS runs |

---

# 8. Tracking, limits and safety

| Concern | Where | Behaviour |
|---|---|---|
| Session counters | manager.go Status | `jobsCompleted`, `jobsFailed` |
| Bandwidth | node/bandwidth.go | page + upload bytes; resets at local midnight; over cap → skip jobs |
| Pause schedule | node/schedule.go | daily window; supports overnight wrap; equal bounds = disabled |
| Concurrency | manager.go:131 | worker pool, default 3 (Settings) |
| Queue backpressure | manager.go:27 | 24 slots; overflow dropped |
| Rate of work | — | dropped jobs simply go to other nodes; no penalty |
| Audit trail | `~/.config/Mellowtel/mellowtel.log` | structured line per event |

**Why "job queue full" floods the log:** Mellowtel offers ~10+ jobs/sec; 3 workers
at 1–8s per browser job absorb ~1/sec. Everything else overflows. This is
**correct backpressure**, not an error — the alternative is unbounded memory
growth. Raising *Max concurrent jobs* to 6–8 converts more of them into work.

**Graceful shutdown:** Disconnect cancels the run context, stopping workers,
socket and poller, then stops Chrome and closes the log. One subtlety: gorilla's
`ReadMessage` ignores context cancellation, so we explicitly close the connection
to unblock it (**wsclient.go:161-167**) — without that, shutdown took **68
seconds**; now it's **25 ms**.

---

# 9. Build, packaging and running

## What happens during a build

1. `npm run build` → Vite compiles React/TS into `frontend/dist`
2. Wails generates JS bindings by **compiling and running a host binary** that
   introspects the `App` struct
3. Go compiles everything, embedding `frontend/dist` via `//go:embed`
4. Output: **one self-contained native executable**

## Linux

Needs native GUI dev libraries because the UI uses WebKitGTK:

```bash
sudo apt install -y libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config
make dev            # live reload
make build-linux    # -> dist/linux/mellowtel-consumer
```

- **Ubuntu 24.04+ ships WebKit 4.1**, but Wails defaults to 4.0 — builds must
  pass **`-tags webkit2_41`** (the Makefile does). Without it the native build
  fails and `wails dev` silently falls back to a **browser** dev server.
- **VS Code snap gotcha:** its integrated terminal injects `GTK_PATH`/`SNAP_*`
  vars that send the binary into snap's old glibc → `symbol lookup error:
  libpthread ... __libc_pthread_init`. Use a normal terminal, or
  `make dev-clean` / `scripts/dev.sh`, which scrub those variables.

## Windows

```bash
make build-windows            # -> dist/windows/mellowtel-consumer.exe (14 MB)
make build-windows-installer  # NSIS installer (needs: apt install nsis)
```

- **Cross-compiles from Linux — verified working**, but requires
  **`-skipbindings`**. Wails' binding step runs a host binary, and a Windows
  `.exe` can't execute on Linux (`fork/exec /tmp/wailsbindings: exec format
  error`). It reuses the committed bindings in `frontend/wailsjs/`, so run
  `wails dev` once after changing any bound method.
- **Runtime dependency:** the **WebView2 runtime** — preinstalled on Windows 11
  and current Windows 10; otherwise Wails prompts, or bundle with `-webview2 embed`.
- Tray works. Autostart uses `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

## macOS

**Cannot be cross-compiled from Linux** — needs Xcode CLT and the darwin SDK.
Build on a Mac:

```bash
xcode-select --install
brew install go node
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
cd frontend && npm install && cd ..
make build-mac        # -> dist/mac/Mellowtel.app (universal: Intel + Apple Silicon)
make build-mac-dmg    # -> dist/mac/Mellowtel.dmg
```

- **Gatekeeper** blocks unsigned apps ("developer cannot be verified"). Personal
  use: right-click → Open, or `xattr -cr`. Distribution needs an Apple Developer
  ID, `codesign`, and notarisation.
- **Known limitation:** macOS requires the tray to own the **main thread**, but
  Wails already does. The tray runs on a goroutine (correct on Linux/Windows), so
  **it may not appear on macOS**. Window and scraping work normally.

## CI

`.github/workflows/build.yml` — runs tests, then builds on **native** Linux,
Windows and macOS runners (so no `-skipbindings` workaround needed) and uploads
all three artifacts. This is the recommended way to produce Mac builds.

---

# 10. Testing — how we prove it actually scrapes

```bash
make test                                    # fast, hermetic unit tests
MELLOWTEL_LIVE=1 go test ./internal/node/ -run TestEndToEndScrape -v
```

The end-to-end test runs the **real** Manager (real Chrome, real pipeline)
against local stand-ins for all three Mellowtel endpoints, and asserts:

- the **browser job** sees JS-injected content → a real browser executed JavaScript
- the **fetch job** sees only the raw placeholder → the simple path is plain HTTP
- results carry the correct `node_identifier`, status 200, non-empty markdown
- shutdown is prompt (regression guard for the 68s hang)

The marker string is assembled at runtime from two halves so it never appears in
the page source — otherwise a plain fetch would "find" it and prove nothing.

There's also `go run ./testserver` for watching a full cycle live, and a gated
live browser test against real sites.

---

# 11. Code map

| Path | Responsibility |
|---|---|
| `main.go` | Wails entry, window config, tray wiring, bootstrap |
| `app.go` | Methods exposed to the frontend; lifecycle hooks |
| `internal/config/` | config.json load/save; **all endpoint URLs** |
| `internal/device/` | Persistent node identifier |
| `internal/logging/` | zerolog → stderr + file |
| `internal/wsclient/` | **ws.mellow.tel** — registration, heartbeat, reconnect |
| `internal/approval/` | **api.mellow.tel/approval** polling |
| `internal/job/` | Job schema, wire-quirk parser, routing decision |
| `internal/fetch/` | Plain HTTP path |
| `internal/browser/` | Chrome detection, CDP engine, actions |
| `internal/markdown/` | HTML → Markdown |
| `internal/result/` | **request.mellow.tel** result POST |
| `internal/executor/` | One job end-to-end |
| `internal/node/` | Manager: lifecycle, workers, bandwidth, schedule |
| `internal/tray/` | System tray |
| `internal/autostart/` | Launch-on-login per OS |
| `frontend/src/` | React UI |
| `testserver/` | Local Mellowtel stand-in |

---

# 12. Meeting cheat sheet

**"What did you build?"**
A cross-platform desktop client that lets users share their bandwidth and earn —
like Grass.io. Go backend, Wails for the desktop shell, React UI. It connects to
Mellowtel over WebSocket, receives scraping jobs, runs them in the user's own
Chrome, and returns the extracted content.

**"Why Go and Wails instead of Electron?"**
Wails uses the OS's native webview instead of bundling Chromium — 14 MB vs
~150 MB. We need a real Chrome for scraping anyway, so bundling a second browser
for the UI would be waste. Go gives us a single static binary and easy concurrency.

**"Does it use the user's browser or its own?"**
Their installed Chrome — we never bundle a browser. But we launch a **separate
headless instance with an isolated profile**, so their cookies, logins and
browsing are never touched. We deliberately don't attach to a Chrome they're
already running.

**"How does it talk to Mellowtel?"**
Three endpoints. A persistent WebSocket (`ws.mellow.tel`) that delivers jobs —
registration data rides as query params, there's no JSON handshake. An approval
API polled every 30 min that can revoke the node. And an HTTPS POST to
`request.mellow.tel` carrying the scraped content, stamped with our device ID.

**"How does scraping actually work?"**
Job arrives → we route it: plain HTTP if it doesn't need JS, otherwise a new tab
in headless Chrome over CDP. We navigate, wait, run any scripted actions, grab
the rendered `outerHTML`, convert to Markdown, and POST it back.

**"How do you know it works?"**
An automated end-to-end test runs the real pipeline against a page whose content
is injected by JavaScript. The browser path sees it; the plain-fetch path doesn't.
That proves a real browser really executed the page.

**"What are the limitations?"**
No anti-bot evasion — Cloudflare-protected sites will fail. No earnings ledger
(that's backend). No screenshot job types. macOS tray needs a main-thread fix.
And one machine can't absorb the full job firehose, so we apply backpressure and
drop excess jobs — they just go to other nodes.

**"Is it safe / private for the user?"**
Sharing is opt-in via an explicit Connect. Scraping runs in an isolated browser
profile that never touches their real one. The CDP port is loopback-only. Users
can cap daily bandwidth and set pause hours. Everything is logged locally.
The honest caveat: traffic exits from their home IP, which is the core of the
business model and should be clearly disclosed to users.
