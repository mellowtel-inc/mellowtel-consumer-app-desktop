package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/browser"
	"mellowtel-consumer/internal/fetch"
	"mellowtel-consumer/internal/job"
	"mellowtel-consumer/internal/markdown"
	"mellowtel-consumer/internal/result"
)

type stubRenderer struct {
	html string
}

func (s *stubRenderer) Render(_ context.Context, req *job.Request) (*browser.RenderResult, error) {
	return &browser.RenderResult{HTML: s.html, FinalURL: req.URL, StatusCode: 200, Bytes: int64(len(s.html))}, nil
}

func newExec(t *testing.T, renderer Renderer, endpoint string) *Executor {
	t.Helper()
	return New(Config{
		Log:             zerolog.Nop(),
		Fetcher:         fetch.New(),
		Renderer:        renderer,
		Markdown:        markdown.New(),
		Submitter:       result.New(zerolog.Nop()),
		NodeID:          "mllwtl_consumer_test123456",
		DefaultEndpoint: endpoint,
	})
}

func TestExecuteBrowserJobSubmitsMarkdown(t *testing.T) {
	var captured result.Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("bad body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := newExec(t, &stubRenderer{html: "<h1>Hi</h1><p>there</p>"}, srv.URL)

	req, err := job.Parse([]byte(`{"recordID":"rec-9","url":"https://x.com","saveMarkdown":true,"saveHtml":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.SaveHTMLEndpoint = srv.URL

	out, err := exec.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Path != job.PathBrowser {
		t.Errorf("path = %v, want browser", out.Path)
	}
	if captured.RecordID != "rec-9" {
		t.Errorf("recordID = %q", captured.RecordID)
	}
	if captured.NodeIdentifier != "mllwtl_consumer_test123456" {
		t.Errorf("node_identifier = %q", captured.NodeIdentifier)
	}
	if captured.MarkDown == nil || *captured.MarkDown == "" {
		t.Errorf("expected markdown content, got %v", captured.MarkDown)
	}
	if captured.Content == nil || *captured.Content == "" {
		t.Errorf("expected html content when saveHtml set")
	}
}

func TestExecuteSimpleFetchJob(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body><h2>Fetched</h2></body></html>"))
	}))
	defer target.Close()

	var captured result.Payload
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	// No renderer: browser path would fall back, but fetchInstead forces simple.
	exec := newExec(t, nil, sink.URL)

	req, err := job.Parse([]byte(`{"recordID":"rec-s","url":"` + target.URL + `","fetchInstead":true,"saveMarkdown":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.SaveHTMLEndpoint = sink.URL

	out, err := exec.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Path != job.PathSimple {
		t.Errorf("path = %v, want simple", out.Path)
	}
	if captured.MarkDown == nil || *captured.MarkDown == "" {
		t.Errorf("expected markdown from fetched html")
	}
}
