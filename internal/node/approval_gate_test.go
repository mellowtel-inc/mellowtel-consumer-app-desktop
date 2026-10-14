package node

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/job"
)

func gateManager(t *testing.T, endpoint string) *Manager {
	t.Helper()
	cfg := config.Defaults()
	cfg.Endpoints.ApprovalURL = endpoint + "/approval"
	cfg.Endpoints.WebSocketURL = "ws" + strings.TrimPrefix(endpoint, "http") + "/socket"
	m := NewManager(zerolog.Nop(), &cfg, t.TempDir(), "mllwtl_consumer_test")
	m.chromeFound = false // All tests use local servers and never launch Chrome.
	m.SetDeviceToken("device-proof")
	t.Cleanup(m.Disconnect)
	return m
}

func TestConnectRequiresApprovalBeforeOpeningSocket(t *testing.T) {
	for _, outcome := range []string{"denied", "unavailable", "malformed", "missing credential", "approved"} {
		t.Run(outcome, func(t *testing.T) {
			var approvalCalls, socketCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/approval" {
					approvalCalls.Add(1)
					if r.Header.Get("X-Earnbear-Device-Token") != "device-proof" {
						t.Error("missing device proof")
					}
					switch outcome {
					case "unavailable":
						http.Error(w, "offline", 503)
					case "malformed":
						_, _ = w.Write([]byte("{}"))
					case "approved":
						_, _ = w.Write([]byte(`{"approval":true}`))
					default:
						_, _ = w.Write([]byte(`{"approval":false}`))
					}
					return
				}
				socketCalls.Add(1)
				if approvalCalls.Load() == 0 {
					t.Error("socket opened before approval")
				}
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			m := gateManager(t, server.URL)
			if outcome == "missing credential" {
				m.SetDeviceToken("")
			}
			err := m.Connect()
			if outcome == "approved" {
				if err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(time.Second)
				for socketCalls.Load() == 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if socketCalls.Load() == 0 || !m.Status().Approved {
					t.Fatal("approved device did not connect")
				}
				m.onApproval(false, errors.New("approval offline"))
				if m.IsConnected() || m.Status().Approved || !m.Status().Paused {
					t.Fatal("approval failure did not pause immediately")
				}
			} else {
				if err == nil {
					t.Fatal("unapproved connect succeeded")
				}
				if socketCalls.Load() != 0 || m.IsConnected() || m.Status().Approved || !m.Status().Paused {
					t.Fatal("unapproved node started work")
				}
			}
			m.Disconnect()
		})
	}
}

func TestDisconnectCancelsInitialApproval(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	m := gateManager(t, server.URL)
	done := make(chan error, 1)
	go func() { done <- m.Connect() }()
	<-entered
	m.DisconnectAsync()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled startup succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("approval request was not canceled")
	}
	m.Disconnect()
	if m.IsConnected() || m.Status().Approved {
		t.Fatal("canceled startup remained active")
	}
}

func TestUnapprovedJobsAreNeverFetched(t *testing.T) {
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetches.Add(1) }))
	defer server.Close()
	m := gateManager(t, server.URL)
	m.runJob(context.Background(), m.buildExecutor(nil), &job.Request{URL: server.URL, ReceivedAt: time.Now()})
	if fetches.Load() != 0 || m.Status().JobsCompleted != 0 {
		t.Fatal("unapproved node executed a job")
	}
}

func TestCheckApprovalNeverStartsSharing(t *testing.T) {
	for _, result := range []string{"approved", "not_approved", "unavailable"} {
		t.Run(result, func(t *testing.T) {
			var sockets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/approval" {
					sockets.Add(1)
					http.Error(w, "unexpected socket", 500)
					return
				}
				switch result {
				case "approved":
					_, _ = w.Write([]byte(`{"approval":true}`))
				case "not_approved":
					_, _ = w.Write([]byte(`{"approval":false}`))
				default:
					http.Error(w, "approval offline", 503)
				}
			}))
			defer server.Close()
			m := gateManager(t, server.URL)
			err := m.CheckApproval(context.Background())
			if result == "approved" && err != nil {
				t.Fatal(err)
			}
			if result != "approved" && err == nil {
				t.Fatal("denial check succeeded")
			}
			if m.Status().SharingAccess != result {
				t.Fatalf("sharing access = %q, want %q", m.Status().SharingAccess, result)
			}
			if m.IsConnected() || !m.Status().Paused || sockets.Load() != 0 {
				t.Fatal("checking permission started sharing")
			}
		})
	}
}
