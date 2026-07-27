package job

import "testing"

func TestParseBasicJob(t *testing.T) {
	data := []byte(`{
		"recordID": "rec-1",
		"url": "https://example.com",
		"saveHtml": true,
		"saveMarkdown": false,
		"waitBeforeScraping": 2,
		"save_html_endpoint": "https://request.mellow.tel/"
	}`)
	req, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !req.IsJob() {
		t.Fatalf("expected IsJob() true")
	}
	if req.RecordID != "rec-1" || req.URL != "https://example.com" {
		t.Errorf("unexpected fields: %+v", req)
	}
	if !req.SaveHTML || req.SaveMarkdown {
		t.Errorf("save flags wrong: html=%v md=%v", req.SaveHTML, req.SaveMarkdown)
	}
	if req.WaitBeforeScraping != 2 {
		t.Errorf("waitBeforeScraping = %v, want 2", req.WaitBeforeScraping)
	}
}

func TestParseStringEncodedFields(t *testing.T) {
	// Wire quirks: booleans as strings, screen sizes as "Npx", actions as a
	// JSON-encoded string.
	data := []byte(`{
		"recordID": "rec-2",
		"url": "https://example.com",
		"saveHtml": "true",
		"screen_width": "1280px",
		"screen_height": "720px",
		"actions": "[{\"type\":\"wait\",\"milliseconds\":500},{\"type\":\"click\",\"selector\":\".more\"}]"
	}`)
	req, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !req.SaveHTML {
		t.Errorf("expected saveHtml true from string 'true'")
	}
	if req.ScreenWidth != 1280 || req.ScreenHeight != 720 {
		t.Errorf("screen size = %dx%d, want 1280x720", req.ScreenWidth, req.ScreenHeight)
	}
	if len(req.Actions) != 2 {
		t.Fatalf("actions len = %d, want 2", len(req.Actions))
	}
	if req.Actions[0].Type != "wait" || req.Actions[0].Milliseconds != 500 {
		t.Errorf("action[0] = %+v", req.Actions[0])
	}
	if req.Actions[1].Type != "click" || req.Actions[1].Selector != ".more" {
		t.Errorf("action[1] = %+v", req.Actions[1])
	}
}

func TestParseActionsAsArray(t *testing.T) {
	data := []byte(`{"recordID":"r","url":"https://x.com","actions":[{"type":"scroll","direction":"down","amount":800}]}`)
	req, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(req.Actions) != 1 || req.Actions[0].Type != "scroll" || req.Actions[0].Amount != 800 {
		t.Errorf("actions = %+v", req.Actions)
	}
}

func TestParseControlFrames(t *testing.T) {
	req, err := Parse([]byte(`{"type_event":"heartbeat"}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !req.IsControlFrame() || req.IsJob() {
		t.Errorf("heartbeat should be a control frame, not a job")
	}
}

func TestParseDefaults(t *testing.T) {
	req, err := Parse([]byte(`{"recordID":"r","url":"https://x.com"}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !req.SaveMarkdown {
		t.Errorf("saveMarkdown should default to true")
	}
	if req.WaitBeforeScraping != 1 {
		t.Errorf("waitBeforeScraping default = %v, want 1", req.WaitBeforeScraping)
	}
	if req.ScreenWidth != 1024 || req.ScreenHeight != 768 {
		t.Errorf("default screen = %dx%d, want 1024x768", req.ScreenWidth, req.ScreenHeight)
	}
	if !req.FastLane {
		t.Errorf("fastLane should default true")
	}
}
