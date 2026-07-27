# How it works, end to end

This is the complete lifecycle: how the app identifies itself, connects to
Mellowtel, receives jobs, executes them, sends results back, and tracks it all.

---

## 0. Startup (before anything network happens)

`main.go: bootstrap()` runs in order:

1. **Config** — `internal/config` loads `config.json` from the OS config dir
   (Linux `~/.config/Mellowtel/`), writing defaults on first run. Every endpoint
   lives here; nothing is hardcoded elsewhere.
2. **Logging** — `internal/logging` sets up zerolog writing to both stderr and
   `mellowtel.log` in that same dir.
3. **Identity** — `internal/device` loads or creates the node ID, persisted in
   the file `device_id`. Format: `mllwtl_<integration>_<rand10>` — e.g. your
   `mllwtl_consumer_5apf20hmt2`. It is **stable across restarts**; that string is
   how Mellowtel knows which node did the work.
4. **Manager** — `internal/node.NewManager` builds the orchestrator and runs
   **Chrome detection** immediately (`internal/browser.FindChrome`) so the UI can
   warn if Chrome is missing.

Then Wails opens the window. Nothing has touched the network yet — the node is
idle until you press **Connect** (or `autoConnect` is on).

---

## 1. Connect

`App.Connect()` → `node.Manager.Connect()` (`internal/node/manager.go`). It:

1. Creates a cancellable **run context** that owns everything started below —
   cancelling it is how Disconnect cleanly stops the whole machine.
2. **Launches one headless Chrome** (`browser.Engine.Start`) using an isolated
   profile at `~/.config/Mellowtel/chrome-profile/`, so your real browsing
   profile is never touched. One browser process for the app's lifetime.
3. Starts a **worker pool** (`maxConcurrentJobs`, default 3) reading from a
   **bounded job queue** (24 slots).
4. Opens the **WebSocket** and starts **approval polling**.

If Chrome fails to launch, this is non-fatal: the engine is nil and jobs fall
back to the plain-HTTP path.

---

## 2. Connecting to Mellowtel (the WebSocket)

`internal/wsclient`. There is **no JSON handshake** — all registration data is
carried as **query parameters on the connection URL**:

```
wss://ws.mellow.tel?device_id=mllwtl_consumer_5apf20hmt2
                   &version=700.0.29
                   &platform=desktop-linux
                   &speed_download=500
```

Once connected the client is purely reactive: it never sends job traffic
upstream (results go over HTTPS, not the socket).

**Heartbeat.** The client sends a WebSocket protocol **ping every 60s** and
expects a pong; the read deadline (70s) trips if the server goes silent, which
drops the connection and triggers a reconnect. Application-level
`{"type_event":"heartbeat"}` frames from the server are ignored by design.

**Reconnect.** Exponential backoff (5s → 60s cap) in `helpers.go`, looping until
the run context is cancelled. The UI shows `Connecting…` / `Reconnecting…`.

**Control frames.** `{"type_event":"disconnect_device"}` makes the node pause
itself.

---

## 3. Approval

`internal/approval` polls `GET https://api.mellow.tel/approval` (device_id,
version, platform, speed) **immediately on connect, then every 30 minutes**. It
reads `{"approval": true|false}`.

- `true` → keep running.
- `false` → status becomes **Approval failed** and the node disconnects.
- Network error → *ignored* (a flaky connection shouldn't kill a healthy node);
  only an explicit denial pauses it.

---

## 4. Receiving a job

Every inbound text frame hits `Manager.onMessage`:

1. **Parse** (`internal/job.Parse`). The wire format is loosely typed, so the
   parser normalises real quirks: `actions` arrives as a **JSON-encoded string**,
   `screen_width`/`height` look like `"1024px"`, and booleans may be `"true"`
   strings. Defaults match the SDK (`saveMarkdown` true, `waitBeforeScraping` 1,
   1024×768, `fastLane` true).
2. **Filter** — control frames and malformed messages are dropped.
3. **Gate** (`shouldSkip`): skip if inside the **pause schedule** window or over
   the **daily bandwidth cap**.
4. **Enqueue** — non-blocking send into the 24-slot queue. If full, the job is
   dropped with `job queue full; dropping job`.

> **Why dropping is correct:** Mellowtel firehoses far more work than one machine
> can absorb. A bounded queue is deliberate backpressure — the alternative is
> unbounded memory growth. Raise **Max concurrent jobs** in Settings to take more.

---

## 5. Executing a job

A worker picks it up → `internal/executor.Execute`. First, **routing**
(`job.Decide`, pure and unit-tested):

- `fetchInstead: true`, or a custom `method_endpoint` → **simple path**
- otherwise → **browser path** (so client-side JS actually runs)

### Simple path (`internal/fetch`)
Plain `net/http` GET/POST with a Chrome-like User-Agent, 30s timeout, capped at
25 MB. Returns body, status, final URL (after redirects), and bytes read.

### Browser path (`internal/browser`)
1. **New tab** in the already-running Chrome (`chromedp.NewContext(browserCtx)`).
   Never a second browser — that would collide on the profile's `SingletonLock`.
2. The tab's CDP session is **materialised against the tab context** before any
   short-lived child context is derived. (Getting this wrong binds the target to
   a context that later gets cancelled, killing the tab and hanging every
   subsequent action — the bug that caused the 45s timeouts.)
3. **Navigate**, under its own 25s budget. If navigation overruns, we **salvage
   the partial DOM** rather than fail — many pages keep loading trackers forever
   but already have usable content.
4. Wait `waitBeforeScraping`, then optional `waitForElement`.
5. Run **actions** in order (`actions.go`): wait, click, write, press,
   fill_input/textarea/select/form, scroll, waitFor. Missing selectors are
   non-fatal so one bad selector can't sink a good scrape.
6. Extract `document.documentElement.outerHTML` and the final URL.
7. Close the tab (browser stays up for reuse).

Optional CSP/X-Frame-Options stripping via the CDP `Fetch` domain exists but is
**off by default** — top-level navigations aren't subject to those headers, and
interception pauses every response. Enable via `stripFramingHeaders` in config.

---

## 6. Sending results back

`internal/result`. The payload mirrors the Electron SDK's `saveCrawl` exactly:

- **Where:** the job's `save_html_endpoint`, defaulting to
  `https://request.mellow.tel/`
- **How:** `POST` with `Content-Type: text/plain` (the body is JSON despite the
  content type — this matches the SDK)
- **Body:** `recordID`, `node_identifier` (your device ID — this is the
  attribution key), `url`, `final_url`, `statusCode`, `website_unreachable`,
  `fastLane`, `orgId`, `htmlTransformer`, `saveHtml/saveMarkdown/saveText`,
  `requestMessageInfo` (the original job echoed back), and:
  - `content` — raw HTML, **only if** `saveHtml`
  - `markDown` — converted via `internal/markdown`, **only if** `saveMarkdown`

  Both are sent **uncompressed and unencoded**.

Failed renders still submit, with `website_unreachable: true` — reporting a
failure is part of the contract.

**Timeout isolation:** submission runs on a context detached from the job
deadline (`context.WithoutCancel`) with its own 30s budget. Otherwise a slow page
would consume the job budget and starve the POST, losing an already-scraped
result.

---

## 7. Tracking

**In-memory, per session** — `node.Status`, pushed to the UI:

| Field | Meaning |
|---|---|
| `connection` | disconnected / connecting / connected |
| `detail` | human status line ("Connected — N jobs completed") |
| `approved` | last approval result |
| `chromeFound` | Chrome detection result |
| `deviceId` | your node identifier |
| `jobsCompleted` / `jobsFailed` | session counters |
| `bytesUsedToday` | rolling daily traffic |

**Bandwidth accounting** (`bandwidth.go`): every job adds page bytes + result
bytes; usage resets at local midnight; over the cap, jobs are skipped.

**How the UI stays current:** the backend emits a `status:update` Wails event on
every change; the frontend subscribes (retrying until the runtime is injected)
**and** polls `GetStatus()` every 2s as a backstop.

**Persistent record:** `mellowtel.log` has a structured line for every
connection change, job queued/executed/completed/failed, approval check and
error. **Settings → View logs** opens it.

> Earnings show `$0.00` — the ledger is a backend service, deliberately out of
> MVP scope. `jobsCompleted` is the real unit of work for now.

---

## 8. Shutdown

Quit → `App.shutdown` → `Manager.Disconnect`: cancel the run context (stops
workers, socket, approval poller), wait for goroutines, stop Chrome, close the
log file. Closing the window only **hides to tray** when `closeToTray` is on.

---

## One-line summary

`Connect` → register over WebSocket with a persistent device ID → get approved →
receive JSON jobs → route to plain-fetch or headless-Chrome render → convert to
HTML/Markdown → `POST` back to `request.mellow.tel` stamped with the device ID →
count it in the UI and the log.
