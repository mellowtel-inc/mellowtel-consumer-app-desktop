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
	JobsCompleted int64 `json:"jobsCompleted"`
	JobsFailed    int64 `json:"jobsFailed"`
	BytesUsed     int64 `json:"bytesUsed"`
	// EarnedMicroUSD is lifetime earnings in millionths of a dollar. It stays
	// zero until the earnings ledger backend exists — we never invent a figure.
	EarnedMicroUSD int64 `json:"earnedMicroUsd"`
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
