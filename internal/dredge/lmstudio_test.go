package dredge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLMStudioSummarize_Success(t *testing.T) {
	var gotPath string
	var gotReq lmStudioRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		resp := map[string]any{
			"output": []map[string]any{
				{"content": "SUMMARY: A richer bookmark summary.\nTAGS: bookmarks, ai, research"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewLMStudioClient(srv.URL, "test-model")
	summary, tags, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if gotPath != "/api/v1/chat" {
		t.Fatalf("path = %q, want /api/v1/chat", gotPath)
	}
	if gotReq.Model != "test-model" {
		t.Errorf("model = %q", gotReq.Model)
	}
	if gotReq.SystemPrompt == "" {
		t.Error("system_prompt should be set")
	}
	if !strings.Contains(gotReq.Input, "Title: Title") {
		t.Errorf("input did not include title: %q", gotReq.Input)
	}
	if summary != "A richer bookmark summary." {
		t.Errorf("summary = %q", summary)
	}
	if len(tags) != 3 || tags[0] != "bookmarks" {
		t.Errorf("tags = %v, want [bookmarks ai research]", tags)
	}
}

func TestLMStudioSummarize_OpenAIStyleResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "SUMMARY: OpenAI-shaped response.\nTAGS: compat, chat",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewLMStudioClient(srv.URL, "test-model")
	summary, tags, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if summary != "OpenAI-shaped response." {
		t.Errorf("summary = %q", summary)
	}
	if len(tags) != 2 || tags[0] != "compat" {
		t.Errorf("tags = %v, want [compat chat]", tags)
	}
}

func TestLMStudioSummarize_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	client := NewLMStudioClient(srv.URL, "test-model")
	_, _, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err == nil {
		t.Fatal("expected server error")
	}
}

func TestLMStudioSummarize_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	defer srv.Close()

	client := NewLMStudioClient(srv.URL, "test-model")
	_, _, err := client.Summarize(context.Background(), "Title", "Desc", "https://example.com", nil)
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestLMStudioPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("path = %q, want /api/v1/models", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewLMStudioClient(srv.URL, "test-model")
	if !client.Ping() {
		t.Fatal("Ping() = false, want true")
	}
}
