// Package device generates and persists the Mellowtel node identifier.
//
// The identifier format mirrors the Mellowtel SDKs:
//
//	mllwtl_<integration>_<random>
//
// where <random> is up to 10 base-36 characters. The identifier is persisted
// so a node keeps a stable identity across restarts. If the integration key
// changes, the random suffix is preserved and only the key segment rewritten,
// matching the SDK's getOrGenerateIdentifier behaviour.
package device

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

const (
	prefix       = "mllwtl"
	randomLength = 10
	base36       = "0123456789abcdefghijklmnopqrstuvwxyz"
	fileName     = "device_id"
)

// randomSuffix returns a cryptographically random base-36 string of length n.
func randomSuffix(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(base36)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate random suffix: %w", err)
		}
		b[i] = base36[idx.Int64()]
	}
	return string(b), nil
}

// Generate builds a fresh identifier for the given integration key.
func Generate(integration string) (string, error) {
	suffix, err := randomSuffix(randomLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s_%s", prefix, integration, suffix), nil
}

// suffixOf extracts the random suffix (everything after the integration key)
// from an existing identifier, or "" if it cannot be parsed.
func suffixOf(id string) string {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) == 3 && parts[0] == prefix {
		return parts[2]
	}
	return ""
}

// rewriteIntegration returns id with its integration segment replaced,
// preserving the random suffix.
func rewriteIntegration(id, integration string) (string, bool) {
	suffix := suffixOf(id)
	if suffix == "" {
		return "", false
	}
	return fmt.Sprintf("%s_%s_%s", prefix, integration, suffix), true
}

// matchesIntegration reports whether id already belongs to the integration key.
func matchesIntegration(id, integration string) bool {
	return strings.HasPrefix(id, fmt.Sprintf("%s_%s_", prefix, integration))
}

// GetOrCreate loads the persisted identifier from dir, generating and saving a
// new one if absent. If a stored identifier uses a different integration key,
// its random suffix is preserved and the key segment rewritten.
func GetOrCreate(dir, integration string) (string, error) {
	path := filepath.Join(dir, fileName)

	data, err := os.ReadFile(path)
	if err == nil {
		stored := strings.TrimSpace(string(data))
		if matchesIntegration(stored, integration) {
			return stored, nil
		}
		if rewritten, ok := rewriteIntegration(stored, integration); ok {
			if err := save(path, rewritten); err != nil {
				return "", err
			}
			return rewritten, nil
		}
		// Stored value is unparseable; fall through and regenerate.
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read device id %q: %w", path, err)
	}

	id, err := Generate(integration)
	if err != nil {
		return "", err
	}
	if err := save(path, id); err != nil {
		return "", err
	}
	return id, nil
}

func save(path, id string) error {
	if err := os.WriteFile(path, []byte(id), 0o644); err != nil {
		return fmt.Errorf("persist device id %q: %w", path, err)
	}
	return nil
}
