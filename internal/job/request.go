// Package job defines the incoming scrape-job schema and the router that
// decides how each job is executed.
package job

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Action is a single browser interaction step within a job.
type Action struct {
	Type         string       `json:"type"`
	Milliseconds int          `json:"milliseconds,omitempty"`
	Selector     string       `json:"selector,omitempty"`
	Value        string       `json:"value,omitempty"`
	Text         string       `json:"text,omitempty"`
	Key          string       `json:"key,omitempty"`
	Direction    string       `json:"direction,omitempty"`
	Amount       int          `json:"amount,omitempty"`
	Timeout      int          `json:"timeout,omitempty"`
	Fields       []FormField  `json:"fields,omitempty"`
}

// FormField is a name/value pair used by the fill_form action.
type FormField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Request is a parsed scrape job received over the WebSocket.
type Request struct {
	RecordID           string
	URL                string
	OrgID              string
	Method             string // GET_NORMAL, GET, POST, ...
	SaveHTML           bool
	SaveMarkdown       bool
	SaveText           bool
	HTMLTransformer    string
	WaitBeforeScraping float64 // seconds
	Actions            []Action
	FetchInstead       bool
	HTMLVisualizer     bool
	HTMLContained      bool
	SaveHTMLEndpoint   string
	FastLane           bool
	SkipHeaders        bool
	WaitForElement     string
	WaitForElementTime int
	RemoveImages       bool
	ScreenWidth        int
	ScreenHeight       int
	MethodEndpoint     string
	MethodPayload      string
	MethodHeaders      map[string]string

	// TypeEvent distinguishes control frames ("heartbeat", "batch", ...) from
	// job messages (empty).
	TypeEvent string

	// Raw holds the original decoded message so it can be echoed back in the
	// result payload as requestMessageInfo.
	Raw map[string]json.RawMessage

	// ReceivedAt is when this job arrived over the socket. Jobs that sit in the
	// queue too long may be reassigned or expired server-side, so age matters
	// when diagnosing rejected submissions.
	ReceivedAt time.Time
}

// rawMap is a helper for reading loosely-typed JSON fields.
type rawMap map[string]json.RawMessage

func (m rawMap) has(key string) bool {
	_, ok := m[key]
	return ok
}

// str reads a field as a string, tolerating that it may already be a JSON
// string or a bare token.
func (m rawMap) str(key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// boolOr reads a field that may be a JSON bool or a "true"/"false" string.
func (m rawMap) boolOr(key string, def bool) bool {
	raw, ok := m[key]
	if !ok {
		return def
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseBool(strings.TrimSpace(s)); err == nil {
			return v
		}
	}
	return def
}

// floatOr reads a field that may be a JSON number or a numeric string.
func (m rawMap) floatOr(key string, def float64) float64 {
	raw, ok := m[key]
	if !ok {
		return def
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return v
		}
	}
	return def
}

func (m rawMap) intOr(key string, def int) int {
	return int(m.floatOr(key, float64(def)))
}

// parseSize turns "1024px" (or "1024") into an int.
func parseSize(s string, def int) int {
	s = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(s), "px"))
	if s == "" {
		return def
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return int(v)
	}
	return def
}

// parseActions decodes the actions field, which the wire encodes as a
// JSON-string containing an array (though a bare array is also tolerated).
func parseActions(raw json.RawMessage) ([]Action, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// First try: the value is itself a JSON array.
	var actions []Action
	if err := json.Unmarshal(raw, &actions); err == nil {
		return actions, nil
	}
	// Otherwise it is a JSON string containing a JSON array.
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("actions field is neither array nor string: %w", err)
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "no_actions" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(s), &actions); err != nil {
		return nil, fmt.Errorf("parse actions string: %w", err)
	}
	return actions, nil
}

// parseHeaders decodes method_headers, a JSON-string object or "no_headers".
func parseHeaders(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	headers := map[string]string{}
	if err := json.Unmarshal(raw, &headers); err == nil {
		return headers
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	if s == "" || s == "no_headers" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), &headers); err != nil {
		return nil
	}
	return headers
}

// Parse decodes a raw WebSocket text frame into a Request. It never returns a
// nil Request on success. Control frames (heartbeat, etc.) parse successfully
// with TypeEvent set and an empty URL.
func Parse(data []byte) (*Request, error) {
	var raw rawMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode job envelope: %w", err)
	}

	r := &Request{
		Raw:                raw,
		ReceivedAt:         time.Now(),
		TypeEvent:          raw.str("type_event"),
		RecordID:           raw.str("recordID"),
		URL:                raw.str("url"),
		OrgID:              raw.str("orgId"),
		Method:             raw.str("method"),
		SaveHTML:           raw.boolOr("saveHtml", false),
		SaveMarkdown:       raw.boolOr("saveMarkdown", true),
		SaveText:           raw.boolOr("saveText", false),
		HTMLTransformer:    raw.str("htmlTransformer"),
		WaitBeforeScraping: raw.floatOr("waitBeforeScraping", 1),
		FetchInstead:       raw.boolOr("fetchInstead", false),
		HTMLVisualizer:     raw.boolOr("htmlVisualizer", false),
		HTMLContained:      raw.boolOr("htmlContained", false),
		SaveHTMLEndpoint:   raw.str("save_html_endpoint"),
		FastLane:           raw.boolOr("fastLane", true),
		SkipHeaders:        raw.boolOr("skipHeaders", false),
		WaitForElement:     raw.str("waitForElement"),
		WaitForElementTime: raw.intOr("waitForElementTime", 0),
		RemoveImages:       raw.boolOr("removeImages", false),
		MethodEndpoint:     raw.str("method_endpoint"),
		MethodPayload:      raw.str("method_payload"),
	}

	if r.HTMLTransformer == "" {
		r.HTMLTransformer = "none"
	}
	if r.Method == "" {
		r.Method = "GET_NORMAL"
	}
	r.ScreenWidth = parseSize(raw.str("screen_width"), 1024)
	r.ScreenHeight = parseSize(raw.str("screen_height"), 768)
	r.MethodHeaders = parseHeaders(raw["method_headers"])

	actions, err := parseActions(raw["actions"])
	if err != nil {
		return nil, err
	}
	r.Actions = actions

	return r, nil
}

// IsControlFrame reports whether the message is a control event rather than a
// scrape job.
func (r *Request) IsControlFrame() bool {
	switch r.TypeEvent {
	case "heartbeat", "disconnect_device", "refresh_cereal":
		return true
	}
	return false
}

// IsJob reports whether the message is an executable scrape job.
func (r *Request) IsJob() bool {
	return !r.IsControlFrame() && r.URL != "" && r.RecordID != ""
}
