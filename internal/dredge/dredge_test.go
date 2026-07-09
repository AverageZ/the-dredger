package dredge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/logging"
	"github.com/alexzajac/the-dredger/internal/model"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func htmlPage(title, description string) string {
	return fmt.Sprintf(`<html><head>
		<title>%s</title>
		<meta name="description" content="%s">
	</head><body></body></html>`, title, description)
}

func newTestService(database *sql.DB, workers int, llm LLMClient) *Service {
	svc := NewService(database, workers, llm, logging.Nop())
	svc.SetHostDelay(0)
	return svc
}

func TestService_RunEmpty(t *testing.T) {
	database := setupTestDB(t)
	svc := newTestService(database, 2, NewOllamaClient("http://invalid:0", ""))

	go svc.Run(context.Background(), nil)

	count := 0
	for range svc.Results() {
		count++
	}
	if count != 0 {
		t.Errorf("got %d results, want 0", count)
	}
}

func TestService_RunSingleLink(t *testing.T) {
	// Set up a test HTTP server that serves an HTML page
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlPage("Test Title", "Test Description"))
	}))
	defer srv.Close()

	database := setupTestDB(t)
	link := model.Link{URL: srv.URL + "/page"}
	id, err := db.InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id

	// Use invalid ollama URL so it skips LLM (ollama not available)
	svc := newTestService(database, 1, NewOllamaClient("http://invalid:0", ""))
	go svc.Run(context.Background(), []model.Link{link})

	var results []Result
	for r := range svc.Results() {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].Title != "Test Title" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Test Title")
	}
	if results[0].Description != "Test Description" {
		t.Errorf("Description = %q, want %q", results[0].Description, "Test Description")
	}
}

func TestService_RunMultipleLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlPage("Page", "Desc"))
	}))
	defer srv.Close()

	database := setupTestDB(t)
	var links []model.Link
	for i := range 3 {
		l := model.Link{URL: fmt.Sprintf("%s/page%d", srv.URL, i)}
		id, err := db.InsertLink(database, l)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		l.ID = id
		links = append(links, l)
	}

	svc := newTestService(database, 2, NewOllamaClient("http://invalid:0", ""))
	go svc.Run(context.Background(), links)

	count := 0
	for r := range svc.Results() {
		if r.Err != nil {
			t.Errorf("result %d error: %v", count, r.Err)
		}
		count++
	}
	if count != 3 {
		t.Errorf("got %d results, want 3", count)
	}
}

func TestService_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlPage("Page", "Desc"))
	}))
	defer srv.Close()

	database := setupTestDB(t)
	var links []model.Link
	for i := range 10 {
		l := model.Link{URL: fmt.Sprintf("%s/page%d", srv.URL, i)}
		id, err := db.InsertLink(database, l)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		l.ID = id
		links = append(links, l)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	svc := newTestService(database, 2, NewOllamaClient("http://invalid:0", ""))
	go svc.Run(ctx, links)

	count := 0
	for range svc.Results() {
		count++
	}
	// With immediate cancellation, we should get fewer than all 10
	if count >= 10 {
		t.Errorf("got %d results with cancelled context, expected fewer than 10", count)
	}
}

func TestService_FetchError(t *testing.T) {
	// Server that returns 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	database := setupTestDB(t)
	link := model.Link{URL: srv.URL + "/missing"}
	id, err := db.InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id

	svc := newTestService(database, 1, NewOllamaClient("http://invalid:0", ""))
	go svc.Run(context.Background(), []model.Link{link})

	result := <-svc.Results()
	if result.LinkID != id {
		t.Errorf("LinkID = %d, want %d", result.LinkID, id)
	}
	if result.Err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

func TestService_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "slow down")
	}))
	defer srv.Close()

	database := setupTestDB(t)
	link := model.Link{URL: srv.URL + "/limited"}
	id, err := db.InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id

	svc := newTestService(database, 1, NewOllamaClient("http://invalid:0", ""))
	go svc.Run(context.Background(), []model.Link{link})

	result := <-svc.Results()
	if result.Err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}

	links, err := db.GetLinks(database)
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if links[0].Enriched {
		t.Fatal("rate-limited link was marked enriched")
	}
	if links[0].DredgeState != model.DredgeCapsized {
		t.Errorf("DredgeState = %v, want Capsized", links[0].DredgeState)
	}
}

func TestService_WithOllama(t *testing.T) {
	// HTML server
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlPage("Test", "A test page"))
	}))
	defer htmlSrv.Close()

	// Mock Ollama server
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/generate" {
			resp := ollamaResponse{Response: "SUMMARY: A test summary.\nTAGS: test, mock"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ollamaSrv.Close()

	database := setupTestDB(t)
	link := model.Link{URL: htmlSrv.URL + "/page"}
	id, err := db.InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id

	svc := newTestService(database, 1, NewOllamaClient(ollamaSrv.URL, "test-model"))
	go svc.Run(context.Background(), []model.Link{link})

	result := <-svc.Results()
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Summary != "A test summary." {
		t.Errorf("Summary = %q, want %q", result.Summary, "A test summary.")
	}
	if len(result.Tags) != 2 || result.Tags[0] != "test" {
		t.Errorf("Tags = %v, want [test mock]", result.Tags)
	}
}

func TestService_WithLMStudio(t *testing.T) {
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlPage("Test", "A test page"))
	}))
	defer htmlSrv.Close()

	lmStudioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v1/chat" {
			resp := map[string]any{
				"output": []map[string]any{
					{"content": "SUMMARY: An LM Studio summary.\nTAGS: lmstudio, mock"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer lmStudioSrv.Close()

	database := setupTestDB(t)
	link := model.Link{URL: htmlSrv.URL + "/page"}
	id, err := db.InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id

	svc := newTestService(database, 1, NewLMStudioClient(lmStudioSrv.URL, "test-model"))
	go svc.Run(context.Background(), []model.Link{link})

	result := <-svc.Results()
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Summary != "An LM Studio summary." {
		t.Errorf("Summary = %q, want %q", result.Summary, "An LM Studio summary.")
	}
	if len(result.Tags) != 2 || result.Tags[0] != "lmstudio" {
		t.Errorf("Tags = %v, want [lmstudio mock]", result.Tags)
	}
}
