package result

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
)

func newTestSubmitter() *Submitter {
	return New(zerolog.Nop())
}

// The endpoint intermittently returns 500 "Error saving content to S3" for
// payloads it otherwise accepts, so a transient failure must be retried.
func TestSubmitRetriesTransient500(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":true,"message":"Error saving content to S3"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestSubmitter()
	if _, err := s.Submit(context.Background(), srv.URL, Payload{RecordID: "r1"}); err != nil {
		t.Fatalf("expected success on 3rd attempt, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestSubmitGivesUpAfterMaxAttempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":true,"message":"Error saving content to S3"}`))
	}))
	defer srv.Close()

	s := newTestSubmitter()
	_, err := s.Submit(context.Background(), srv.URL, Payload{RecordID: "r2"})
	if err == nil {
		t.Fatal("expected failure after exhausting attempts")
	}
	if got := atomic.LoadInt32(&calls); got != maxAttempts {
		t.Errorf("attempts = %d, want %d", got, maxAttempts)
	}
}

// A 4xx means the payload itself is wrong; retrying only wastes bandwidth.
func TestSubmitDoesNotRetryClientError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad payload"}`))
	}))
	defer srv.Close()

	s := newTestSubmitter()
	if _, err := s.Submit(context.Background(), srv.URL, Payload{RecordID: "r3"}); err == nil {
		t.Fatal("expected failure")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestSubmitSucceedsFirstTry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestSubmitter()
	sent, err := s.Submit(context.Background(), srv.URL, Payload{RecordID: "r4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent <= 0 {
		t.Errorf("sent bytes = %d, want > 0", sent)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

// The server's explanation must reach the log — that message is what turned
// "status 500" into a diagnosable problem.
func TestSubmitErrorIncludesServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"very specific reason"}`))
	}))
	defer srv.Close()

	s := newTestSubmitter()
	_, err := s.Submit(context.Background(), srv.URL, Payload{RecordID: "r5"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "very specific reason") {
		t.Errorf("error should surface the server message, got: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
