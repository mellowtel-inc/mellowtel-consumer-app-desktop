package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSignInPersistsAndRestoresSession(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "earnbear_access", Value: "access-token", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "earnbear_refresh", Value: "refresh-token", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case "/api/auth/session":
			cookie, err := r.Cookie("earnbear_access")
			if err != nil || cookie.Value != "access-token" {
				_ = json.NewEncoder(w).Encode(map[string]any{"configured": true, "user": nil})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"configured": true,
				"user":       map[string]any{"email": "bear@example.com", "emailVerified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL(dir, server.URL)
	state, err := client.SignIn("bear@example.com", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Authenticated || state.Email != "bear@example.com" || !state.EmailVerified {
		t.Fatalf("unexpected state: %+v", state)
	}
	if !client.HasSession() {
		t.Fatal("signed-in client should have a local session")
	}

	info, err := os.Stat(filepath.Join(dir, "account-session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session permissions = %o, want 600", info.Mode().Perm())
	}

	restored := NewWithBaseURL(dir, server.URL)
	restoredState, err := restored.GetState()
	if err != nil || !restoredState.Authenticated {
		t.Fatalf("restored state = %+v, err = %v", restoredState, err)
	}
	if err := restored.SignOut(); err != nil {
		t.Fatal(err)
	}
	if restored.HasSession() {
		t.Fatal("signed-out client should not have a local session")
	}
	if _, err := os.Stat(filepath.Join(dir, "account-session.json")); !os.IsNotExist(err) {
		t.Fatalf("session file still exists after sign out: %v", err)
	}
}

func TestSignUpAndConfirm(t *testing.T) {
	var signup, confirm bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/signup":
			signup = true
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "confirmed": false})
		case "/api/auth/confirm":
			confirm = true
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL(t.TempDir(), server.URL)
	result, err := client.SignUp("new@example.com", "strong-password")
	if err != nil || result.Confirmed {
		t.Fatalf("signup result = %+v, err = %v", result, err)
	}
	if err := client.ConfirmSignUp("new@example.com", "123456"); err != nil {
		t.Fatal(err)
	}
	if !signup || !confirm {
		t.Fatal("signup and confirm endpoints were not both called")
	}
}

func TestRegisterDeviceUsesAuthenticatedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices/register" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie("earnbear_access")
		if err != nil || cookie.Value != "access-token" {
			t.Fatalf("device registration missing account session")
		}
		var payload DeviceRegistration
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.DeviceID != "mllwtl_consumer_abc123" || payload.Platform != "darwin" {
			t.Fatalf("unexpected registration: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "deviceToken": "device-proof", "linkedAt": "2026-08-11T10:00:00Z",
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	client := NewWithBaseURL(dir, server.URL)
	client.session.Cookies["earnbear_access"] = "access-token"
	result, err := client.RegisterDevice(DeviceRegistration{
		DeviceID: "mllwtl_consumer_abc123", AppVersion: "0.1.0", ProtocolVersion: "700.0.29",
		Platform: "darwin", Integration: "consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeviceToken != "device-proof" {
		t.Fatalf("device token = %q", result.DeviceToken)
	}
}
