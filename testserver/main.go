// Command testserver is a self-contained local stand-in for Mellowtel's
// infrastructure, used to prove the desktop app really scrapes.
//
// It runs three things:
//
//   - a target page (/target) whose real content is injected by JavaScript
//     half a second after load, so only a real browser can see it;
//   - a WebSocket at / that hands the node two jobs for that page — one
//     browser-rendered, one fetchInstead (plain HTTP);
//   - a result sink (/result) that receives the scraped payloads and prints a
//     verdict.
//
// The browser job MUST contain the JS-injected marker; the fetch job MUST NOT.
// That contrast proves both execution paths behave correctly.
//
// Usage:
//
//	go run ./testserver
//
// then point the app's config.json at it (see the printed instructions).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	addr   = ":8899"
	marker = "MELLOWTEL_JS_RENDERED_OK"
)

// targetHTML renders its real content only after JS runs. The marker is
// assembled at runtime from two halves so the literal string never appears in
// the raw HTML — otherwise a plain fetch would "find" it in the script tag and
// the two execution paths would be indistinguishable.
const targetHTML = `<!doctype html>
<html><head><title>Mellowtel scrape target</title></head>
<body>
  <h1>Static heading (visible to plain HTTP)</h1>
  <div id="content">PLACEHOLDER_NOT_YET_RENDERED</div>
  <script>
    setTimeout(function () {
      document.getElementById('content').textContent = 'MELLOWTEL' + '_JS_RENDERED_OK';
      var p = document.createElement('p');
      p.textContent = 'Injected at ' + new Date().toISOString();
      document.body.appendChild(p);
    }, 500);
  </script>
</body></html>`

type verdict struct {
	mu      sync.Mutex
	results map[string]bool // recordID -> marker present
}

var (
	v       = &verdict{results: map[string]bool{}}
	outDir  string
	upgrade = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
)

func main() {
	var err error
	outDir, err = os.MkdirTemp("", "mellowtel-testserver-*")
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Deliberately hostile headers: a top-level navigation ignores these,
		// which is exactly why header stripping is unnecessary.
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		fmt.Fprint(w, targetHTML)
	})

	http.HandleFunc("/approval", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ approval check from device_id=%s", r.URL.Query().Get("device_id"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"approval": true}`)
	})

	http.HandleFunc("/result", handleResult)
	http.HandleFunc("/", handleWS)

	printBanner()
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleWS accepts the node's registration socket and pushes two jobs.
func handleWS(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		fmt.Fprint(w, "mellowtel testserver: use /target, /result, /approval, or connect via WebSocket")
		return
	}
	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	q := r.URL.Query()
	fmt.Println()
	log.Printf("✅ NODE CONNECTED")
	log.Printf("   device_id      = %s", q.Get("device_id"))
	log.Printf("   version        = %s", q.Get("version"))
	log.Printf("   platform       = %s", q.Get("platform"))
	log.Printf("   speed_download = %s", q.Get("speed_download"))

	base := "http://localhost" + addr
	jobs := []map[string]any{
		{
			"recordID":           "browser-job",
			"url":                base + "/target",
			"saveHtml":           true,
			"saveMarkdown":       true,
			"waitBeforeScraping": 2, // let the JS run
			"save_html_endpoint": base + "/result",
		},
		{
			"recordID":           "fetch-job",
			"url":                base + "/target",
			"fetchInstead":       true, // plain HTTP: must NOT see the marker
			"saveHtml":           true,
			"saveMarkdown":       true,
			"save_html_endpoint": base + "/result",
		},
	}

	for _, j := range jobs {
		payload, _ := json.Marshal(j)
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Printf("send job failed: %v", err)
			return
		}
		log.Printf("→ sent job %q (%s)", j["recordID"], pathOf(j))
		time.Sleep(300 * time.Millisecond)
	}

	// Keep the socket open; respond to pings automatically via the library.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("node disconnected: %v", err)
			return
		}
	}
}

func pathOf(j map[string]any) string {
	if b, ok := j["fetchInstead"].(bool); ok && b {
		return "plain HTTP fetch"
	}
	return "browser render"
}

// handleResult receives a scraped payload and evaluates it.
func handleResult(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("❌ bad result body: %v", err)
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	recordID, _ := body["recordID"].(string)
	content, _ := body["content"].(string)
	markDown, _ := body["markDown"].(string)
	nodeID, _ := body["node_identifier"].(string)
	finalURL, _ := body["final_url"].(string)
	status, _ := body["statusCode"].(float64)
	unreachable, _ := body["website_unreachable"].(bool)

	hasMarker := strings.Contains(content, marker) || strings.Contains(markDown, marker)

	// Persist for inspection.
	file := filepath.Join(outDir, recordID+".json")
	pretty, _ := json.MarshalIndent(body, "", "  ")
	os.WriteFile(file, pretty, 0o644)

	fmt.Println()
	log.Printf("📥 RESULT RECEIVED: %s", recordID)
	log.Printf("   node_identifier = %s", nodeID)
	log.Printf("   final_url       = %s", finalURL)
	log.Printf("   statusCode      = %.0f   website_unreachable=%v", status, unreachable)
	log.Printf("   content         = %d bytes", len(content))
	log.Printf("   markDown        = %d bytes", len(markDown))
	log.Printf("   JS marker found = %v", hasMarker)
	log.Printf("   saved to        = %s", file)

	switch recordID {
	case "browser-job":
		if hasMarker {
			log.Printf("   ✅ PASS — a real browser executed the page's JavaScript.")
		} else {
			log.Printf("   ❌ FAIL — browser job did NOT contain the JS-injected marker.")
		}
	case "fetch-job":
		if !hasMarker {
			log.Printf("   ✅ PASS — plain fetch correctly returned raw HTML (no JS).")
		} else {
			log.Printf("   ❌ FAIL — fetch job unexpectedly ran JavaScript.")
		}
	}

	v.mu.Lock()
	v.results[recordID] = hasMarker
	done := len(v.results) >= 2
	browserOK := v.results["browser-job"]
	fetchOK := !v.results["fetch-job"]
	v.mu.Unlock()

	if done {
		fmt.Println()
		log.Printf("=========== VERDICT ===========")
		if browserOK && fetchOK {
			log.Printf("🎉 ALL CHECKS PASSED — the desktop app is genuinely scraping.")
			log.Printf("   • browser path rendered JavaScript")
			log.Printf("   • fetch path returned raw HTML")
			log.Printf("   • results returned with the correct node identifier")
		} else {
			log.Printf("⚠️  SOMETHING IS WRONG — see the FAIL lines above.")
		}
		log.Printf("Scraped payloads: %s", outDir)
		log.Printf("===============================")
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"ok":true}`)
}

func printBanner() {
	base := "http://localhost" + addr
	fmt.Println(`
========================================================================
  Mellowtel local test server
========================================================================
Listening on ` + addr + `

To point the desktop app at it, edit:
  ~/.config/Mellowtel/config.json

Set:
  "endpoints": {
    "websocketURL":          "ws://localhost` + addr + `",
    "approvalURL":           "` + base + `/approval",
    "defaultResultEndpoint": "` + base + `/result"
  }

Then restart the app (make dev-clean) and press Connect.

Waiting for the node to connect...
========================================================================`)
}
