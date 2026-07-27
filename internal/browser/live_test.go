package browser

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/job"
)

// TestLiveRender exercises the real browser engine against real sites. It is
// skipped unless MELLOWTEL_LIVE=1 so CI and normal `go test` runs stay hermetic.
//
//	MELLOWTEL_LIVE=1 go test ./internal/browser/ -run TestLiveRender -v
func TestLiveRender(t *testing.T) {
	if os.Getenv("MELLOWTEL_LIVE") != "1" {
		t.Skip("set MELLOWTEL_LIVE=1 to run live browser tests")
	}

	dir := t.TempDir()
	log := zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.DebugLevel)
	eng := NewEngine(log, "", dir)
	if err := eng.Start(); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	defer eng.Stop()

	urls := []string{
		"https://example.com",
		"https://www.eurotopics.net/de/148780/sueddeutsche-zeitung",
		"https://gearjunkie.com/motors/best-all-terrain-tires",
	}

	for _, u := range urls {
		for _, strip := range []bool{false, true} {
			label := "default"
			if strip {
				label = "strip-headers"
			}
			t.Run(label+" "+u, func(t *testing.T) {
				// Toggle the engine's interception mode so both the default
				// (off) and opt-in (on) paths are actually covered.
				eng.SetStripHeaders(strip)
				req := &job.Request{
					RecordID:           "live",
					URL:                u,
					SaveMarkdown:       true,
					WaitBeforeScraping: 1,
				}
				start := time.Now()
				res, err := eng.Render(context.Background(), req)
				elapsed := time.Since(start).Round(time.Millisecond)
				if err != nil {
					t.Errorf("FAIL  %-14s %-60s  %v  err=%v", label, u, elapsed, err)
					return
				}
				t.Logf("OK    %-14s %-60s  %v  bytes=%d status=%d", label, u, elapsed, len(res.HTML), res.StatusCode)
			})
		}
	}
}
