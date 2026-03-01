package dredge

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

type Result struct {
	LinkID      int64
	Title       string
	Description string
	Summary     string
	Tags        []string
	Err         error
}

type Service struct {
	db      *sql.DB
	client  *http.Client
	ollama  *OllamaClient
	workers int
	results chan Result
}

func NewService(database *sql.DB, workers int) *Service {
	return &Service{
		db: database,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		ollama:  NewOllamaClient("", ""),
		workers: workers,
		results: make(chan Result, workers*2),
	}
}

func (s *Service) Results() <-chan Result {
	return s.results
}

func (s *Service) Run(ctx context.Context, links []model.Link) {
	if len(links) == 0 {
		close(s.results)
		return
	}

	type job struct {
		id  int64
		url string
	}

	jobs := make(chan job, len(links))
	for _, l := range links {
		jobs <- job{id: l.ID, url: l.URL}
	}
	close(jobs)

	ollamaAvailable := s.ollama.Ping()

	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}

				// Set state to crawling
				db.UpdateDredgeState(s.db, j.id, model.DredgeCrawling, "")

				delay := time.Duration(200+rand.IntN(600)) * time.Millisecond
				time.Sleep(delay)

				result := s.fetchOne(ctx, j.id, j.url)
				if result.Err != nil {
					_ = db.UpdateDredgeState(s.db, j.id, model.DredgeCapsized, fmt.Sprintf("crawl: %s", result.Err.Error()))
				} else if !ollamaAvailable {
					// Ollama not running — save crawl data, skip crunch
					_ = db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, "", nil)
				} else {
					// Crunching phase: LLM summarization
					db.UpdateDredgeState(s.db, j.id, model.DredgeCrunching, "")
					summary, tags, err := s.ollama.Summarize(ctx, result.Title, result.Description, j.url)
					if err != nil {
						// Crawl succeeded but crunch failed — still save crawl data
						_ = db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, "", nil)
						_ = db.UpdateDredgeState(s.db, j.id, model.DredgeCapsized, err.Error())
						result.Err = err
					} else {
						result.Summary = summary
						result.Tags = tags
						_ = db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, summary, tags)
					}
				}

				select {
				case s.results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	wg.Wait()
	close(s.results)
}

func (s *Service) fetchOne(ctx context.Context, id int64, url string) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{LinkID: id, Err: fmt.Errorf("create request: %w", err)}
	}
	req.Header.Set("User-Agent", "TheDredger/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return Result{LinkID: id, Err: fmt.Errorf("fetch %s: %w", url, err)}
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 1<<20) // 1MB
	meta := ScrapeMetadata(limited)

	title := meta.Title
	if title == "" {
		title = url
	}

	return Result{
		LinkID:      id,
		Title:       title,
		Description: meta.Description,
	}
}
