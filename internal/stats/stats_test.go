package stats

import (
	"os"
	"testing"
)

func TestRecordAndPersist(t *testing.T) {
	dir := t.TempDir()

	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.RecordSuccess(1000)
	s.RecordSuccess(500)
	s.RecordFailure(100)

	got := s.Totals()
	if got.JobsCompleted != 2 || got.JobsFailed != 1 {
		t.Errorf("counters = %+v", got)
	}
	if got.BytesUsed != 1600 {
		t.Errorf("bytes = %d, want 1600", got.BytesUsed)
	}

	// Totals must survive a reload — this is what makes the UI cumulative.
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Totals() != got {
		t.Errorf("after reload = %+v, want %+v", reloaded.Totals(), got)
	}
}

func TestSuccessRate(t *testing.T) {
	if (Totals{}).SuccessRate() != 0 {
		t.Errorf("empty totals should report 0%%, not NaN")
	}
	tot := Totals{JobsCompleted: 99, JobsFailed: 1}
	if got := tot.SuccessRate(); got < 98.9 || got > 99.1 {
		t.Errorf("SuccessRate() = %v, want ~99", got)
	}
}

func TestEarningsStayZero(t *testing.T) {
	// Earnings must never be invented; they stay zero until a ledger exists.
	dir := t.TempDir()
	s, _ := Load(dir)
	s.RecordSuccess(5000)
	if s.Totals().EarnedUSD() != 0 {
		t.Errorf("earnings should remain 0 without a ledger backend")
	}
}

func TestCorruptFileDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir+"/"+fileName, "{not json"); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("corrupt stats should not error: %v", err)
	}
	if s.Totals().JobsCompleted != 0 {
		t.Errorf("corrupt stats should start from zero")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
