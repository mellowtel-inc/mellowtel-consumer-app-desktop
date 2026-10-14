package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"mellowtel-consumer/internal/account"
	"mellowtel-consumer/internal/activity"
	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/node"
)

func TestAccountSwitchRequiresNewApprovalAndKeepsInstallationProof(t *testing.T) {
	var mu sync.Mutex
	approved := map[string]bool{"account-a": true, "account-b": false}
	linked := ""
	registrations, approvalChecks := 0, 0
	proof := strings.Repeat("x", 43)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		cookie, _ := r.Cookie("earnbear_access")
		owner := ""
		if cookie != nil {
			owner = cookie.Value
		}
		switch r.URL.Path {
		case "/api/auth/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			http.SetCookie(w, &http.Cookie{Name: "earnbear_access", Value: body["email"], Path: "/"})
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/auth/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"sub": owner, "email": owner, "emailVerified": true}, "earningApproved": approved[owner]})
		case "/api/devices/register":
			registrations++
			var body account.DeviceRegistration
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !approved[owner] || (linked != "" && body.InstallationToken != proof) {
				t.Error("registration did not carry the approved session and installation proof")
				w.WriteHeader(403)
				return
			}
			linked = owner
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "deviceToken": "device-proof-" + owner, "installationToken": proof, "userId": owner})
		case "/approval":
			approvalChecks++
			_, _ = w.Write([]byte(`{"approval":true}`))
		default:
			t.Error("preflight started an unexpected network operation", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Endpoints.ApprovalURL = server.URL + "/approval"
	client := account.NewWithBaseURL(dir, server.URL)
	reporter, err := activity.New(dir, client)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: &cfg, manager: node.NewManager(zerolog.Nop(), &cfg, dir, "mllwtl_consumer_test"), account: client, activity: reporter}
	defer a.manager.Disconnect()
	if _, err := a.SignIn("account-a", "password"); err != nil {
		t.Fatal(err)
	}
	if result := a.CheckSharingApproval(); result.SharingAccess != "approved" {
		t.Fatalf("account A preflight: %+v", result)
	}
	if err := a.SignOut(); err != nil {
		t.Fatal(err)
	}
	// Restore the client to exercise installation persistence after logout.
	a.account = account.NewWithBaseURL(dir, server.URL)
	if state, err := a.SignIn("account-b", "password"); err != nil || state.EarningApproved {
		t.Fatalf("unapproved B login: %+v %v", state, err)
	}
	if result := a.CheckSharingApproval(); result.SharingAccess != "not_approved" {
		t.Fatalf("unapproved B preflight: %+v", result)
	}
	mu.Lock()
	if registrations != 1 || linked != "account-a" {
		t.Error("unapproved login transferred the device")
	}
	approved["account-b"] = true
	mu.Unlock()
	if _, err := a.SignIn("account-b", "password"); err != nil {
		t.Fatal(err)
	}
	if result := a.CheckSharingApproval(); result.SharingAccess != "approved" {
		t.Fatalf("approved B preflight: %+v", result)
	}
	if _, err := a.SignIn("account-a", "password"); err != nil {
		t.Fatal(err)
	}
	if result := a.CheckSharingApproval(); result.SharingAccess != "approved" {
		t.Fatalf("return to A preflight: %+v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if linked != "account-a" || registrations != 3 || approvalChecks != 3 || a.manager.IsConnected() {
		t.Fatal("account switch did not finish safely with sharing paused")
	}
}

func TestRegistrationConflictBlocksPreflightAndConnect(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var checks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/session":
					_, _ = w.Write([]byte(`{"user":{"sub":"user-bear","email":"bear@example.com","emailVerified":true},"earningApproved":true}`))
				case "/api/devices/register":
					var registration account.DeviceRegistration
					if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
						t.Error(err)
					}
					if registration.DeviceID != "mllwtl_consumer_test" {
						t.Error("registration changed the persistent device identity")
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"success":false,"message":"This device is revoked or already linked to another account."}`))
				default:
					checks.Add(1)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			cfg := config.Defaults()
			cfg.Endpoints.ApprovalURL = server.URL + "/approval"
			client := account.NewWithBaseURL(dir, server.URL)
			reporter, err := activity.New(dir, client)
			if err != nil {
				t.Fatal(err)
			}
			a := &App{cfg: &cfg, manager: node.NewManager(zerolog.Nop(), &cfg, dir, "mllwtl_consumer_test"), account: client, activity: reporter}
			defer a.manager.Disconnect()
			wantAccess, wantReason := "unavailable", "Could not register this device"
			if status == http.StatusConflict {
				wantAccess, wantReason = "device_conflict", "revoked or linked"
			}
			assertBlocked := func(result node.Status) {
				t.Helper()
				if result.SharingAccess != wantAccess || !strings.Contains(result.Detail, wantReason) || result.Approved || !result.Paused {
					t.Fatalf("registration failure state = %+v", result)
				}
			}
			assertBlocked(a.CheckSharingApproval())
			if err := a.connectRegisteredDevice(); err == nil {
				t.Fatal("sharing started after failed registration")
			}
			assertBlocked(a.manager.Status())
			if checks.Load() != 0 || a.manager.IsConnected() {
				t.Fatal("failed registration reached device approval or started sharing")
			}
		})
	}
}

func TestAppConnectRequiresLiveAccountApproval(t *testing.T) {
	for _, state := range []string{"signed out", "unapproved", "unavailable", "approved"} {
		t.Run(state, func(t *testing.T) {
			var registrations, deviceChecks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/session":
					if state == "unavailable" {
						http.Error(w, "offline", 503)
						return
					}
					var user any = map[string]any{"sub": "user-bear", "email": "bear@example.com", "emailVerified": true}
					if state == "signed out" {
						user = nil
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"user": user, "earningApproved": state == "approved"})
				case "/api/devices/register":
					registrations.Add(1)
					_, _ = w.Write([]byte(`{"success":true,"deviceToken":"proof","installationToken":"install-proof","userId":"user-bear"}`))
				case "/approval":
					deviceChecks.Add(1)
					// Even approved accounts must pass device approval before connecting.
					_, _ = w.Write([]byte(`{"approval":false}`))
				default:
					t.Error("unexpected network request", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			cfg := config.Defaults()
			cfg.Endpoints.ApprovalURL = server.URL + "/approval"
			client := account.NewWithBaseURL(dir, server.URL)
			reporter, err := activity.New(dir, client)
			if err != nil {
				t.Fatal(err)
			}
			a := &App{cfg: &cfg, manager: node.NewManager(zerolog.Nop(), &cfg, dir, "mllwtl_consumer_test"), account: client, activity: reporter}
			defer a.manager.Disconnect()
			checked := a.CheckSharingApproval()
			if checked.SharingAccess != "not_approved" && checked.SharingAccess != "unavailable" {
				t.Fatalf("denied approval check = %+v", checked)
			}
			if a.manager.IsConnected() {
				t.Fatal("approval check started sharing")
			}
			if err := a.connectRegisteredDevice(); err == nil {
				t.Fatal("sharing unexpectedly started")
			}
			want := int32(0)
			if state == "approved" {
				want = 2
			}
			if registrations.Load() != want || deviceChecks.Load() != want {
				t.Fatalf("registration %d device checks %d, want %d", registrations.Load(), deviceChecks.Load(), want)
			}
			if a.manager.IsConnected() || a.manager.Status().Approved {
				t.Fatal("denied account/device remained active")
			}
		})
	}
}
