package browser

import (
	"strings"
	"testing"

	"github.com/chromedp/cdproto/fetch"
)

func TestFilterHeadersRemovesProtective(t *testing.T) {
	in := []*fetch.HeaderEntry{
		{Name: "Content-Type", Value: "text/html"},
		{Name: "X-Frame-Options", Value: "DENY"},
		{Name: "content-security-policy", Value: "default-src 'self'"},
		{Name: "Cross-Origin-Opener-Policy", Value: "same-origin"},
		{Name: "Cache-Control", Value: "no-cache"},
	}
	out := filterHeaders(in)

	for _, h := range out {
		switch strings.ToLower(h.Name) {
		case "x-frame-options", "content-security-policy", "cross-origin-opener-policy":
			t.Errorf("protective header %q was not stripped", h.Name)
		}
	}
	// Non-protective headers must survive.
	if !hasHeader(out, "Content-Type") || !hasHeader(out, "Cache-Control") {
		t.Errorf("benign headers were dropped: %+v", out)
	}
}

func hasHeader(hs []*fetch.HeaderEntry, name string) bool {
	for _, h := range hs {
		if strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

func TestFindChromeReturnsConsistent(t *testing.T) {
	// Should not panic and should be internally consistent (path non-empty iff found).
	path, found := FindChrome()
	if found && path == "" {
		t.Error("found=true but path empty")
	}
	if !found && path != "" {
		t.Error("found=false but path non-empty")
	}
}
