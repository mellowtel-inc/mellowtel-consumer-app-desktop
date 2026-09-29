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

func TestSyncActivityUsesAuthenticatedDeterministicBatch(t *testing.T) {
	var firstBatchID string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "earnbear_access", Value: "access-token", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case "/api/auth/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"configured": true, "user": map[string]any{"email": "bear@example.com", "emailVerified": true}})
		case "/api/rewards/activity":
			cookie, err := r.Cookie("earnbear_access")
			if err != nil || cookie.Value != "access-token" {
				t.Fatal("activity sync did not include the account session")
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["activityCount"] != float64(157) || payload["deviceId"] != "mllwtl_test-device" {
				t.Fatalf("unexpected activity payload: %+v", payload)
			}
			batchID, _ := payload["batchId"].(string)
			if calls == 0 {
				firstBatchID = batchID
			} else if batchID != firstBatchID {
				t.Fatalf("retry batch ID changed: %q != %q", batchID, firstBatchID)
			}
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "accepted": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL(t.TempDir(), server.URL)
	if _, err := client.SignIn("bear@example.com", "password"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := client.SyncActivity("mllwtl_test-device", "0.1.0", "darwin", "consumer", 0, 157, 157, 0, 2048, 2048); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 || firstBatchID == "" {
		t.Fatalf("activity calls = %d, batch = %q", calls, firstBatchID)
	}
}

func TestSignUpAndConfirm(t *testing.T) {
	var signup, confirm bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/signup":
			signup = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["affiliateCode"] != "CREATOR10" {
				t.Fatalf("affiliateCode = %q, want CREATOR10", payload["affiliateCode"])
			}
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
	result, err := client.SignUp("new@example.com", "strong-password", "CREATOR10")
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

func TestWaitlistOnboarding(t *testing.T) {
	var joined, profiled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/waitlist":
			joined = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["email"] != "wait@example.com" || payload["waitlistReferralCode"] != "abc123abc123" {
				t.Fatalf("unexpected waitlist payload: %+v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "onboardingToken": "token", "inviteUrl": "https://earnbear.app/invite/test",
				"progress": map[string]any{"country": "Portugal", "devices": []string{"macos"}, "socialClaims": map[string]bool{"x": true}, "profileCompleted": true, "acceptedInviteCount": 2, "pendingReferralPoints": 1000},
			})
		case "/api/waitlist-profile":
			profiled = true
			var payload struct {
				Email               string          `json:"email"`
				Devices             []string        `json:"devices"`
				SocialClaims        map[string]bool `json:"socialClaims"`
				OnboardingCompleted bool            `json:"onboardingCompleted"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Email != "wait@example.com" || len(payload.Devices) != 1 || payload.Devices[0] != "macos" || !payload.SocialClaims["discord"] || !payload.OnboardingCompleted {
				t.Fatalf("unexpected profile payload: %+v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "socialBonusPoints": 100})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL(t.TempDir(), server.URL)
	result, err := client.JoinWaitlist("wait@example.com", "abc123abc123", "")
	if err != nil || result.OnboardingToken != "token" || result.Progress == nil || result.Progress.AcceptedInviteCount != 2 {
		t.Fatalf("waitlist result = %+v, err = %v", result, err)
	}
	profile, err := client.SaveWaitlistProfile("wait@example.com", "token", "Portugal", []string{"macos"}, false, false, true, true)
	if err != nil || profile.SocialBonusPoints != 100 {
		t.Fatalf("profile result = %+v, err = %v", profile, err)
	}
	if !joined || !profiled {
		t.Fatal("waitlist join and profile endpoints were not both called")
	}
}
