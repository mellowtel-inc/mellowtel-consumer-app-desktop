// Package activity durably batches provisional desktop completion reports.
package activity

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mellowtel-consumer/internal/account"
)

const (
	outboxFileName = "activity-outbox.jsonl"
	maxBatchSize   = 1_000
	maxOutboxSize  = 50_000
	flushInterval  = 6 * time.Hour
	drainInterval  = 30 * time.Second
	initialRetry   = 30 * time.Second
	maxRetry       = 5 * time.Minute
)

type batchSender interface {
	RecordClientActivityBatch(account.ClientActivityBatch) error
}

type queuedActivity struct {
	BatchID  string
	Activity account.ClientActivity
}

type outboxRecord struct {
	Type     string                  `json:"type"`
	BatchID  string                  `json:"batchId"`
	Activity *account.ClientActivity `json:"activity,omitempty"`
}

// Reporter keeps accepted activities on disk until AWS acknowledges their
// batch. Device credentials remain memory-only and are never written to the
// outbox.
type Reporter struct {
	mu       sync.Mutex
	path     string
	sender   batchSender
	pending  []queuedActivity
	openID   string
	openSize int
	deviceID string
	token    string
	wake     chan struct{}
}

func New(configDir string, sender batchSender) (*Reporter, error) {
	r := &Reporter{
		path:   filepath.Join(configDir, outboxFileName),
		sender: sender,
		wake:   make(chan struct{}, 1),
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// Start runs one serialized uploader until the application context closes.
func (r *Reporter) Start(ctx context.Context) {
	go r.run(ctx)
}

// SetCredential enables future flushes using the current in-memory device
// proof. Pending events from an interrupted session retain their IDs.
func (r *Reporter) SetCredential(deviceID, token string) {
	r.mu.Lock()
	r.deviceID = deviceID
	r.token = token
	r.mu.Unlock()
}

// ClearCredential pauses uploads without deleting pending activity.
func (r *Reporter) ClearCredential() {
	r.mu.Lock()
	r.deviceID = ""
	r.token = ""
	r.mu.Unlock()
}

// Enqueue appends one activity to the local write-ahead log before returning.
func (r *Reporter) Enqueue(item account.ClientActivity) error {
	if strings.TrimSpace(item.ActivityID) == "" || item.BytesUsed < 0 {
		return errors.New("invalid provisional activity")
	}
	if strings.TrimSpace(item.OccurredAt) == "" {
		item.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) >= maxOutboxSize {
		return errors.New("provisional activity outbox is full")
	}
	if r.openID == "" {
		batchID, err := newBatchID()
		if err != nil {
			return err
		}
		r.openID = batchID
		r.openSize = 0
	}
	record := outboxRecord{Type: "activity", BatchID: r.openID, Activity: &item}
	if err := r.appendLocked(record); err != nil {
		return err
	}
	r.pending = append(r.pending, queuedActivity{BatchID: r.openID, Activity: item})
	r.openSize++
	if r.openSize == maxBatchSize {
		if err := r.sealOpenLocked(); err != nil {
			return err
		}
		r.signal()
	}
	return nil
}

func (r *Reporter) run(ctx context.Context) {
	timer := time.NewTimer(jitter(flushInterval))
	defer timer.Stop()
	retry := initialRetry
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-timer.C:
		}

		err := r.flushOnce()
		delay := jitter(flushInterval)
		if err != nil {
			var httpError *account.HTTPError
			if errors.As(err, &httpError) && (httpError.StatusCode == 401 || httpError.StatusCode == 403) {
				r.ClearCredential()
				retry = initialRetry
			} else if errors.As(err, &httpError) && httpError.StatusCode >= 400 && httpError.StatusCode < 500 {
				delay = jitter(time.Hour)
				retry = initialRetry
			} else {
				delay = jitter(retry)
				retry = min(retry*2, maxRetry)
			}
		} else {
			retry = initialRetry
			if r.hasSealedBatch() {
				delay = jitter(drainInterval)
			}
		}
		timer.Reset(delay)
	}
}

func (r *Reporter) hasSealedBatch() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending) > 0 && r.pending[0].BatchID != r.openID
}

func (r *Reporter) flushOnce() error {
	r.mu.Lock()
	if len(r.pending) == 0 || r.deviceID == "" || r.token == "" {
		r.mu.Unlock()
		return nil
	}
	batchID := r.pending[0].BatchID
	if batchID == r.openID {
		if err := r.sealOpenLocked(); err != nil {
			r.mu.Unlock()
			return err
		}
	}
	count := 0
	items := make([]account.ClientActivity, 0, maxBatchSize)
	for count < len(r.pending) && count < maxBatchSize && r.pending[count].BatchID == batchID {
		items = append(items, r.pending[count].Activity)
		count++
	}
	deviceID, token := r.deviceID, r.token
	r.mu.Unlock()

	summary := summarize(items)
	if err := r.sender.RecordClientActivityBatch(account.ClientActivityBatch{
		DeviceID: deviceID, DeviceToken: token, BatchID: batchID,
		ActivityCount: len(items), ActivityBytes: summary.bytes,
		FirstOccurred: summary.firstOccurred, LastOccurred: summary.lastOccurred,
		ActivityDigest: summary.digest,
	}); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) < count {
		return errors.New("provisional activity outbox changed during upload")
	}
	for index := range items {
		if r.pending[index].BatchID != batchID || r.pending[index].Activity.ActivityID != items[index].ActivityID {
			return errors.New("provisional activity outbox changed during upload")
		}
	}
	original := r.pending
	r.pending = r.pending[count:]
	if err := r.rewriteLocked(); err != nil {
		r.pending = original
		return err
	}
	return nil
}

type activitySummary struct {
	bytes                       int64
	firstOccurred, lastOccurred string
	digest                      string
}

func summarize(items []account.ClientActivity) activitySummary {
	hash := sha256.New()
	result := activitySummary{}
	for index, item := range items {
		result.bytes += item.BytesUsed
		if index == 0 || item.OccurredAt < result.firstOccurred {
			result.firstOccurred = item.OccurredAt
		}
		if index == 0 || item.OccurredAt > result.lastOccurred {
			result.lastOccurred = item.OccurredAt
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%s\n", item.ActivityID, item.BytesUsed, item.OccurredAt)
	}
	result.digest = fmt.Sprintf("%x", hash.Sum(nil))
	return result
}

func (r *Reporter) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Reporter) load() error {
	file, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open activity outbox: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 16*1024)
	sealed := make(map[string]bool)
	counts := make(map[string]int)
	lastBatchID := ""
	for scanner.Scan() {
		var record outboxRecord
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.BatchID == "" {
			continue
		}
		if record.Type == "seal" {
			sealed[record.BatchID] = true
			continue
		}
		if record.Type == "activity" && record.Activity != nil && record.Activity.ActivityID != "" && record.Activity.BytesUsed >= 0 {
			r.pending = append(r.pending, queuedActivity{BatchID: record.BatchID, Activity: *record.Activity})
			counts[record.BatchID]++
			lastBatchID = record.BatchID
			if len(r.pending) == maxOutboxSize {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read activity outbox: %w", err)
	}
	if lastBatchID != "" && !sealed[lastBatchID] && counts[lastBatchID] < maxBatchSize {
		r.openID = lastBatchID
		r.openSize = counts[lastBatchID]
	}
	return nil
}

func (r *Reporter) appendLocked(record outboxRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open activity outbox: %w", err)
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append activity outbox: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close activity outbox: %w", err)
	}
	return nil
}

func (r *Reporter) sealOpenLocked() error {
	if r.openID == "" {
		return nil
	}
	if err := r.appendLocked(outboxRecord{Type: "seal", BatchID: r.openID}); err != nil {
		return err
	}
	r.openID = ""
	r.openSize = 0
	return nil
}

func (r *Reporter) rewriteLocked() error {
	tmp := r.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create activity outbox: %w", err)
	}
	writer := bufio.NewWriter(file)
	lastBatchID := ""
	for index, item := range r.pending {
		if lastBatchID != "" && item.BatchID != lastBatchID && lastBatchID != r.openID {
			data, marshalErr := json.Marshal(outboxRecord{Type: "seal", BatchID: lastBatchID})
			if marshalErr != nil {
				_ = file.Close()
				return marshalErr
			}
			if _, err = writer.Write(append(data, '\n')); err != nil {
				_ = file.Close()
				return fmt.Errorf("rewrite activity outbox: %w", err)
			}
		}
		activity := item.Activity
		data, marshalErr := json.Marshal(outboxRecord{Type: "activity", BatchID: item.BatchID, Activity: &activity})
		if marshalErr != nil {
			_ = file.Close()
			return marshalErr
		}
		if _, err = writer.Write(append(data, '\n')); err != nil {
			_ = file.Close()
			return fmt.Errorf("rewrite activity outbox: %w", err)
		}
		lastBatchID = item.BatchID
		if index == len(r.pending)-1 && lastBatchID != r.openID {
			data, marshalErr = json.Marshal(outboxRecord{Type: "seal", BatchID: lastBatchID})
			if marshalErr != nil {
				_ = file.Close()
				return marshalErr
			}
			if _, err = writer.Write(append(data, '\n')); err != nil {
				_ = file.Close()
				return fmt.Errorf("rewrite activity outbox: %w", err)
			}
		}
	}
	if err = writer.Flush(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush activity outbox: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close activity outbox: %w", err)
	}
	if err = os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("replace activity outbox: %w", err)
	}
	return os.Chmod(r.path, 0o600)
}

func jitter(base time.Duration) time.Duration {
	spread := base / 5
	if spread <= 0 {
		return base
	}
	return base - spread + time.Duration(mathrand.Int64N(int64(spread*2)+1))
}

func newBatchID() (string, error) {
	value := make([]byte, 32)
	if _, err := cryptorand.Read(value); err != nil {
		return "", fmt.Errorf("create activity batch ID: %w", err)
	}
	return fmt.Sprintf("%x", value), nil
}
