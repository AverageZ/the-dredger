package dredge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseResponse_Normal(t *testing.T) {
	raw := "SUMMARY: This is a great page about Go.\nTAGS: go, programming, cli"
	summary, tags := parseResponse(raw)

	if summary != "This is a great page about Go." {
		t.Errorf("summary = %q, want %q", summary, "This is a great page about Go.")
	}
	if len(tags) != 3 || tags[0] != "go" || tags[1] != "programming" || tags[2] != "cli" {
		t.Errorf("tags = %v, want [go programming cli]", tags)
	}
}

func TestParseResponse_MultilineSummary(t *testing.T) {
	raw := "SUMMARY: Line one.\nLine two of summary.\nTAGS: a, b"
	summary, tags := parseResponse(raw)

	if summary != "Line one.\nLine two of summary." {
		t.Errorf("summary = %q", summary)
	}
	if len(tags) != 2 {
		t.Errorf("tags = %v, want 2 tags", tags)
	}
}

func TestParseResponse_NoTags(t *testing.T) {
	raw := "SUMMARY: Just a summary, no tags here."
	summary, tags := parseResponse(raw)

	if summary != "Just a summary, no tags here." {
		t.Errorf("summary = %q", summary)
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want empty", tags)
	}
}

func TestParseResponse_NoSummary(t *testing.T) {
	raw := "TAGS: orphan, tags"
	summary, tags := parseResponse(raw)

	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	if len(tags) != 2 {
		t.Errorf("tags = %v, want 2 tags", tags)
	}
}

func TestParseResponse_Empty(t *testing.T) {
	summary, tags := parseResponse("")

	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want empty", tags)
	}
}

func TestPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	if !client.Ping() {
		t.Error("Ping() = false, want true")
	}
}

func TestPing_Failure(t *testing.T) {
	// Use a closed server to simulate unreachable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	if client.Ping() {
		t.Error("Ping() = true, want false")
	}
}

func TestSummarize_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Response: "SUMMARY: A great tool for bookmarks.\nTAGS: bookmarks, productivity, cli",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	summary, tags, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if summary != "A great tool for bookmarks." {
		t.Errorf("summary = %q", summary)
	}
	if len(tags) != 3 {
		t.Errorf("tags = %v, want 3 tags", tags)
	}
}

func TestSummarize_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	_, _, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSummarize_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	_, _, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSummarize_WithComments(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]

		resp := ollamaResponse{Response: "SUMMARY: Great.\nTAGS: a"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	_, _, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", []string{"great article", "very useful"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	// Verify the request body contains the comments
	var req ollamaRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req.Prompt == "" {
		t.Fatal("empty prompt")
	}
	if !contains(req.Prompt, "Community Discussion") {
		t.Error("prompt should contain community discussion section")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
