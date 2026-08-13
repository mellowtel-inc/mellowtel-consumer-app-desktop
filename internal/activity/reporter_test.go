package activity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mellowtel-consumer/internal/account"
)

type fakeSender struct {
	batches []account.ClientActivityBatch
	err     error
}

func (f *fakeSender) RecordClientActivityBatch(batch account.ClientActivityBatch) error {
	f.batches = append(f.batches, batch)
	return f.err
}

func TestReporterPersistsAndAcknowledgesBoundedBatches(t *testing.T) {
	dir := t.TempDir()
	sender := &fakeSender{}
	reporter, err := New(dir, sender)
	if err != nil {
		t.Fatal(err)
	}
	reporter.SetCredential("mllwtl_consumer_abc123", "device-proof")
	for index := 0; index < maxBatchSize+5; index++ {
		if err := reporter.Enqueue(account.ClientActivity{
			ActivityID: fmt.Sprintf("activity-%03d", index), BytesUsed: 1024, OccurredAt: "2026-08-12T00:00:00Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := reporter.flushOnce(); err != nil {
		t.Fatal(err)
	}
	if len(sender.batches) != 1 || sender.batches[0].ActivityCount != maxBatchSize {
		t.Fatalf("unexpected batches: %+v", sender.batches)
	}
	if sender.batches[0].BatchID == "" || sender.batches[0].DeviceToken != "device-proof" || len(sender.batches[0].ActivityDigest) != 64 {
		t.Fatalf("batch is missing identity: %+v", sender.batches[0])
	}

	restored, err := New(dir, &fakeSender{})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.pending) != 5 {
		t.Fatalf("restored pending = %d, want 5", len(restored.pending))
	}
	info, err := os.Stat(filepath.Join(dir, outboxFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("outbox permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestReporterRetainsFailedBatchWithStableID(t *testing.T) {
	dir := t.TempDir()
	sender := &fakeSender{err: errors.New("offline")}
	reporter, err := New(dir, sender)
	if err != nil {
		t.Fatal(err)
	}
	reporter.SetCredential("mllwtl_consumer_abc123", "device-proof")
	if err := reporter.Enqueue(account.ClientActivity{ActivityID: "activity-001", BytesUsed: 10}); err != nil {
		t.Fatal(err)
	}
	if err := reporter.flushOnce(); err == nil {
		t.Fatal("expected first upload to fail")
	}
	if err := reporter.Enqueue(account.ClientActivity{ActivityID: "activity-002", BytesUsed: 20}); err != nil {
		t.Fatal(err)
	}
	restoredSender := &fakeSender{err: errors.New("offline")}
	restored, err := New(dir, restoredSender)
	if err != nil {
		t.Fatal(err)
	}
	restored.SetCredential("mllwtl_consumer_abc123", "new-device-proof")
	if err := restored.flushOnce(); err == nil {
		t.Fatal("expected retry to fail")
	}
	if len(restored.pending) != 2 || len(sender.batches) != 1 || len(restoredSender.batches) != 1 {
		t.Fatalf("pending = %d, first batches = %d, restored batches = %d", len(restored.pending), len(sender.batches), len(restoredSender.batches))
	}
	if sender.batches[0].BatchID != restoredSender.batches[0].BatchID {
		t.Fatalf("batch IDs changed across restart: %q != %q", sender.batches[0].BatchID, restoredSender.batches[0].BatchID)
	}
	if restoredSender.batches[0].ActivityCount != 1 || restoredSender.batches[0].ActivityBytes != 10 {
		t.Fatalf("retry regrouped an accepted batch: %+v", restoredSender.batches[0])
	}
	if sender.batches[0].ActivityDigest != restoredSender.batches[0].ActivityDigest {
		t.Fatalf("batch digest changed across retry: %q != %q", sender.batches[0].ActivityDigest, restoredSender.batches[0].ActivityDigest)
	}
}
