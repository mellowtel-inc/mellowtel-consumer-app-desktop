package markdown

import (
	"strings"
	"testing"
)

func TestConvertHeadingAndLink(t *testing.T) {
	c := New()
	out, err := c.Convert(`<h1>Title</h1><p>Hello <a href="https://x.com">world</a></p>`)
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if !strings.Contains(out, "# Title") {
		t.Errorf("expected ATX heading, got: %q", out)
	}
	if !strings.Contains(out, "[world](https://x.com)") {
		t.Errorf("expected markdown link, got: %q", out)
	}
}

func TestConvertList(t *testing.T) {
	c := New()
	out, err := c.Convert(`<ul><li>one</li><li>two</li></ul>`)
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if !strings.Contains(out, "* one") || !strings.Contains(out, "* two") {
		t.Errorf("expected '*' bullet markers, got: %q", out)
	}
}

func TestConvertEmpty(t *testing.T) {
	c := New()
	out, err := c.Convert("")
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output, got: %q", out)
	}
}
