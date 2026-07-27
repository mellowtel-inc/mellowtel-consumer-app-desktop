package node

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/device"
)

const (
	jsMarker    = "MELLOWTEL_JS_RENDERED_OK"
	placeholder = "PLACEHOLDER_NOT_RENDERED"
)

// targetPage only reveals its real content after JavaScript runs. The marker is
// assembled at runtime from two halves so the literal string never appears in
// the raw HTML source — otherwise a plain fetch would "find" it in the script
// tag and the test could not tell the two paths apart.
const targetPage = `<!doctype html><html><head><title>t</title></head><body>
<h1>Static heading</h1>
<div id="content">` + placeholder + `</div>
<script>setTimeout(function(){
  var m = 'MELLOWTEL' + '_JS_RENDERED_OK';
  document.getElementById('content').textContent = m;
},400);</script>
</body></html>`

type captured struct {
	RecordID       string `json:"recordID"`
	Content        string `json:"content"`
	MarkDown       string `json:"markDown"`
	NodeIdentifier string `json:"node_identifier"`
	FinalURL       string `json:"final_url"`
	StatusCode     int    `json:"statusCode"`
	Unreachable    bool   `json:"website_unreachable"`
}

// TestEndToEndScrape drives the full pipeline against local stand-ins for
// Mellowtel: registration socket -> job -> render -> result POST. It proves the
// app actually scrapes, including JavaScript execution.
//
//	MELLOWTEL_LIVE=1 go test ./internal/node/ -run TestEndToEndScrape -v
func TestEndToEndScrape(t *testing.T) {
	if os.Getenv("MELLOWTEL_LIVE") != "1" {
		t.Skip("set MELLOWTEL_LIVE=1 (launches real Chrome)")
	}

	results := make(chan captured, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		io.WriteString(w, targetPage)
	})
	mux.HandleFunc("/approval", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"approval": true}`)
	})
	mux.HandleFunc("/result", func(w http.ResponseWriter, r *http.Request) {
		var c captured
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &c); err != nil {
			t.Errorf("bad result payload: %v", err)
		}
		results <- c
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Registration socket: hands the node one browser job and one fetch job.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		jobs := []map[string]any{
			{
				"recordID": "browser-job", "url": srv.URL + "/target",
				"saveHtml": true, "saveMarkdown": true, "waitBeforeScraping": 2,
				"save_html_endpoint": srv.URL + "/result",
			},
			{
				"recordID": "fetch-job", "url": srv.URL + "/target",
				"fetchInstead": true, "saveHtml": true, "saveMarkdown": true,
				"save_html_endpoint": srv.URL + "/result",
			},
		}
		for _, j := range jobs {
			b, _ := json.Marshal(j)
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer wsSrv.Close()

	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Endpoints.WebSocketURL = "ws" + strings.TrimPrefix(wsSrv.URL, "http")
	cfg.Endpoints.ApprovalURL = srv.URL + "/approval"
	cfg.Endpoints.DefaultResultEndpoint = srv.URL + "/result"

	deviceID, err := device.GetOrCreate(dir, cfg.Integration)
	if err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(zerolog.New(zerolog.NewConsoleWriter()), &cfg, dir, deviceID)
	if !mgr.ChromeFound() {
		t.Skip("chrome not installed")
	}
	t0 := time.Now()
	if err := mgr.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Logf("TIMING connect took %v", time.Since(t0).Round(time.Millisecond))

	got := map[string]captured{}
	deadline := time.After(90 * time.Second)
	for len(got) < 2 {
		select {
		case c := <-results:
			got[c.RecordID] = c
			t.Logf("received %s: content=%dB markdown=%dB status=%d node=%s",
				c.RecordID, len(c.Content), len(c.MarkDown), c.StatusCode, c.NodeIdentifier)
		case <-deadline:
			t.Fatalf("timed out; received %d/2 results: %v", len(got), keys(got))
		}
	}
	t.Logf("TIMING both results in %v", time.Since(t0).Round(time.Millisecond))

	// Time the shutdown explicitly: graceful teardown is a requirement.
	tShutdown := time.Now()
	mgr.Disconnect()
	shutdownTook := time.Since(tShutdown).Round(time.Millisecond)
	t.Logf("TIMING disconnect took %v", shutdownTook)
	if shutdownTook > 15*time.Second {
		t.Errorf("shutdown too slow: %v (graceful shutdown should be prompt)", shutdownTook)
	}

	// The browser job must have executed the page's JavaScript: the marker is
	// present AND the placeholder has been replaced.
	b := got["browser-job"]
	if !strings.Contains(b.Content, jsMarker) && !strings.Contains(b.MarkDown, jsMarker) {
		t.Errorf("browser job did NOT execute JavaScript; marker missing.\ncontent=%.300s", b.Content)
	} else if strings.Contains(b.Content, placeholder) {
		t.Errorf("browser job still shows the pre-render placeholder")
	} else {
		t.Logf("PASS: browser job executed JavaScript (marker present, placeholder replaced)")
	}
	if b.NodeIdentifier != deviceID {
		t.Errorf("node_identifier = %q, want %q", b.NodeIdentifier, deviceID)
	}
	if b.StatusCode != 200 || b.Unreachable {
		t.Errorf("browser job status=%d unreachable=%v", b.StatusCode, b.Unreachable)
	}
	if b.MarkDown == "" {
		t.Errorf("browser job returned no markdown")
	}

	// The fetch job must NOT have executed JavaScript: no marker, and the
	// untouched placeholder is still there.
	f := got["fetch-job"]
	if strings.Contains(f.Content, jsMarker) {
		t.Errorf("fetch job unexpectedly executed JavaScript")
	} else if !strings.Contains(f.Content, placeholder) {
		t.Errorf("fetch job did not return the un-rendered page")
	} else {
		t.Logf("PASS: fetch job returned raw HTML (no JS execution, placeholder intact)")
	}
	if !strings.Contains(f.Content, "Static heading") {
		t.Errorf("fetch job did not return the page HTML")
	}
}

func keys(m map[string]captured) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
