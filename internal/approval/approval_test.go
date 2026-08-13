package approval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestCheckSendsDeviceProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Earnbear-Device-Token") != "device-proof" {
			http.Error(w, "missing device proof", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("device_id") != "mllwtl_consumer_abc123" {
			t.Fatalf("unexpected device id %q", r.URL.Query().Get("device_id"))
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"approval": true})
	}))
	defer server.Close()

	checker := New(zerolog.Nop(), server.URL, Params{
		DeviceID: "mllwtl_consumer_abc123", DeviceToken: "device-proof",
		Version: "700.0.29", Platform: "desktop", SpeedDownload: 500,
	})
	approved, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !approved {
		t.Fatal("expected device to be approved")
	}
}
