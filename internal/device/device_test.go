package device

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var idPattern = regexp.MustCompile(`^mllwtl_consumer_[0-9a-z]{1,10}$`)

func TestGenerateFormat(t *testing.T) {
	id, err := Generate("consumer")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !idPattern.MatchString(id) {
		t.Errorf("id %q does not match expected format", id)
	}
	parts := strings.Split(id, "_")
	if len(parts) != 3 || parts[0] != "mllwtl" || parts[1] != "consumer" {
		t.Errorf("unexpected id segments: %v", parts)
	}
}

func TestGenerateUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := Generate("consumer")
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestGetOrCreatePersists(t *testing.T) {
	dir := t.TempDir()
	id1, err := GetOrCreate(dir, "consumer")
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	id2, err := GetOrCreate(dir, "consumer")
	if err != nil {
		t.Fatalf("GetOrCreate() second error = %v", err)
	}
	if id1 != id2 {
		t.Errorf("identifier not stable: %q != %q", id1, id2)
	}
}

func TestGetOrCreateRewritesIntegrationKeepsSuffix(t *testing.T) {
	dir := t.TempDir()
	// Seed a stored id under a different integration key.
	if err := os.WriteFile(dir+"/device_id", []byte("mllwtl_oldkey_abc123xyz9"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := GetOrCreate(dir, "consumer")
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if id != "mllwtl_consumer_abc123xyz9" {
		t.Errorf("expected suffix preserved with new key, got %q", id)
	}
}
