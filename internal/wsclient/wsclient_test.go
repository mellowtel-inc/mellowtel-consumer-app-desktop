package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func TestConnURLIncludesRegistrationParams(t *testing.T) {
	c := New(Config{
		Log:            zerolog.Nop(),
		BaseURL:        "wss://ws.mellow.tel",
		DeviceID:       "mllwtl_consumer_abc123",
		Version:        "700.0.29",
		PlatformPrefix: "desktop",
		SpeedDownload:  500,
	})
	got, err := c.connURL()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"device_id=mllwtl_consumer_abc123",
		"version=700.0.29",
		"platform=desktop-",
		"speed_download=500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conn URL %q missing %q", got, want)
		}
	}
}

func TestMessageDispatch(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Earnbear-Device-Token") != "device-proof" {
			http.Error(w, "missing device proof", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Send a heartbeat (should be ignored), then a job (should dispatch).
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type_event":"heartbeat"}`))
		conn.WriteMessage(websocket.TextMessage, []byte(`{"recordID":"r1","url":"https://x.com"}`))
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	var mu sync.Mutex
	var jobs [][]byte
	connected := make(chan struct{}, 1)

	c := New(Config{
		Log:            zerolog.Nop(),
		BaseURL:        wsURL,
		DeviceID:       "mllwtl_consumer_x",
		DeviceToken:    "device-proof",
		Version:        "1",
		PlatformPrefix: "desktop",
		OnMessage: func(data []byte) {
			mu.Lock()
			jobs = append(jobs, data)
			mu.Unlock()
		},
		OnState: func(s State) {
			if s == StateConnected {
				select {
				case connected <- struct{}{}:
				default:
				}
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx)
	defer cancel()

	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("never connected")
	}
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 dispatched job (heartbeat filtered), got %d", len(jobs))
	}
	if !strings.Contains(string(jobs[0]), `"recordID":"r1"`) {
		t.Errorf("unexpected dispatched job: %s", jobs[0])
	}
}
