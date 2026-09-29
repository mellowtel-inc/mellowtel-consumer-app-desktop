// Package stats keeps lifetime totals that survive restarts, so the UI can show
// cumulative progress rather than only the current session.
package stats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const fileName = "stats.json"

// Totals is the persisted lifetime record.
type Totals struct {
	JobsCompleted       int64 `json:"jobsCompleted"`
	JobsFailed          int64 `json:"jobsFailed"`
	BytesUsed           int64 `json:"bytesUsed"`
	SyncedJobsCompleted int64 `json:"syncedJobsCompleted,omitempty"`
	SyncedBytesUsed     int64 `json:"syncedBytesUsed,omitempty"`
	// EarnedMicroUSD is lifetime earnings in millionths of a dollar. It stays
	// zero until the earnings ledger backend exists — we never invent a figure.
	EarnedMicroUSD int64 `json:"earnedMicroUsd"`
}

// ActivitySnapshot is one bounded, retry-safe range of locally persisted MVP
// activity waiting to be copied to the user's Earnbear account.
type ActivitySnapshot struct {
	FromJobs      int64
	ToJobs        int64
	ActivityCount int64
	FromBytes     int64
	ToBytes       int64
	ActivityBytes int64
}

// Store persists Totals to disk, flushing after each update.
type Store struct {
	mu     sync.Mutex
	path   string
	totals Totals
}

// Load reads stats.json from dir, starting from zero if absent.
func Load(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName)}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read stats: %w", err)
	}
	if err := json.Unmarshal(data, &s.totals); err != nil {
		// A corrupt stats file must not stop the app; start fresh.
		return s, nil
	}
	return s, nil
}

// Totals returns a snapshot.
func (s *Store) Totals() Totals {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totals
}

// PendingActivity returns at most one API-sized batch. The cursor advances
// only after the server accepts the deterministic batch, so retries are safe.
func (s *Store) PendingActivity() ActivitySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	remainingJobs := s.totals.JobsCompleted - s.totals.SyncedJobsCompleted
	if remainingJobs <= 0 {
		return ActivitySnapshot{}
	}
	count := min(remainingJobs, 1000)
	remainingBytes := max(int64(0), s.totals.BytesUsed-s.totals.SyncedBytesUsed)
	batchBytes := remainingBytes
	if count < remainingJobs {
		batchBytes = remainingBytes * count / remainingJobs
	}
	return ActivitySnapshot{
		FromJobs:      s.totals.SyncedJobsCompleted,
		ToJobs:        s.totals.SyncedJobsCompleted + count,
		ActivityCount: count,
		FromBytes:     s.totals.SyncedBytesUsed,
		ToBytes:       s.totals.SyncedBytesUsed + batchBytes,
		ActivityBytes: batchBytes,
	}
}

// MarkActivitySynced commits the local cursor after the server acknowledges a
// batch. Stale acknowledgements cannot move the cursor backwards.
func (s *Store) MarkActivitySynced(snapshot ActivitySnapshot) {
	s.mu.Lock()
	if snapshot.ToJobs > s.totals.SyncedJobsCompleted {
		s.totals.SyncedJobsCompleted = min(snapshot.ToJobs, s.totals.JobsCompleted)
	}
	if snapshot.ToBytes > s.totals.SyncedBytesUsed {
		s.totals.SyncedBytesUsed = min(snapshot.ToBytes, s.totals.BytesUsed)
	}
	s.mu.Unlock()
	s.save()
}

// RecordSuccess adds a completed job and its traffic.
func (s *Store) RecordSuccess(bytes int64) {
	s.mu.Lock()
	s.totals.JobsCompleted++
	s.totals.BytesUsed += bytes
	s.mu.Unlock()
	s.save()
}

// RecordFailure adds a failed job.
func (s *Store) RecordFailure(bytes int64) {
	s.mu.Lock()
	s.totals.JobsFailed++
	s.totals.BytesUsed += bytes
	s.mu.Unlock()
	s.save()
}

// SuccessRate returns the lifetime success percentage (0 when no jobs yet).
func (t Totals) SuccessRate() float64 {
	total := t.JobsCompleted + t.JobsFailed
	if total == 0 {
		return 0
	}
	return float64(t.JobsCompleted) / float64(total) * 100
}

// EarnedUSD converts micro-USD to dollars.
func (t Totals) EarnedUSD() float64 {
	return float64(t.EarnedMicroUSD) / 1_000_000
}

func (s *Store) save() {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.totals, "", "  ")
	path := s.path
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	os.Rename(tmp, path)
}
