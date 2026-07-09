package dredge

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
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
	Comments    []string
	Err         error
}

type Service struct {
	db      *sql.DB
	client  *http.Client
	llm     LLMClient
	workers int
	results chan Result
	logger  *slog.Logger

	hostDelay time.Duration
	hostMu    sync.Mutex
	lastFetch map[string]time.Time
}

func NewService(database *sql.DB, workers int, llm LLMClient, logger *slog.Logger) *Service {
	return &Service{
		db: database,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		llm:     llm,
		workers: workers,
		results: make(chan Result, workers*2),
		logger:  logger,

		hostDelay: 2 * time.Second,
		lastFetch: make(map[string]time.Time),
	}
}

func (s *Service) SetHostDelay(delay time.Duration) {
	s.hostDelay = delay
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

	llmAvailable := s.llm != nil && s.llm.Ping()

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
				if err := db.UpdateDredgeState(s.db, j.id, model.DredgeCrawling, ""); err != nil {
					s.logger.Error("failed to set crawling state", "link_id", j.id, "err", err)
				}

				delay := time.Duration(200+rand.IntN(600)) * time.Millisecond
				time.Sleep(delay)

				result := s.fetchOne(ctx, j.id, j.url)
				if result.Err != nil {
					if err := db.UpdateDredgeState(s.db, j.id, model.DredgeCapsized, fmt.Sprintf("crawl: %s", result.Err.Error())); err != nil {
						s.logger.Error("failed to set capsized state", "link_id", j.id, "err", err)
					}
				} else if !llmAvailable {
					// LLM service not running — save crawl data, skip crunch
					if err := db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, "", nil); err != nil {
						s.logger.Error("failed to save crawl result", "link_id", j.id, "err", err)
					}
				} else {
					// Crunching phase: LLM summarization
					if err := db.UpdateDredgeState(s.db, j.id, model.DredgeCrunching, ""); err != nil {
						s.logger.Error("failed to set crunching state", "link_id", j.id, "err", err)
					}
					summary, tags, err := s.llm.Summarize(ctx, result.Title, result.Description, j.url, result.Comments)
					if err != nil {
						// Crawl succeeded but crunch failed — still save crawl data
						if dbErr := db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, "", nil); dbErr != nil {
							s.logger.Error("failed to save crawl result after crunch failure", "link_id", j.id, "err", dbErr)
						}
						if dbErr := db.UpdateDredgeState(s.db, j.id, model.DredgeCapsized, err.Error()); dbErr != nil {
							s.logger.Error("failed to set capsized state", "link_id", j.id, "err", dbErr)
						}
						result.Err = err
					} else {
						result.Summary = summary
						result.Tags = tags
						if dbErr := db.UpdateDredgeResult(s.db, j.id, result.Title, result.Description, summary, tags); dbErr != nil {
							s.logger.Error("failed to save dredge result", "link_id", j.id, "err", dbErr)
						}
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

func (s *Service) fetchOne(ctx context.Context, id int64, rawURL string) Result {
	// Resolve aggregator URLs (e.g. HN comments) to article URLs
	resolved, err := ResolveURL(ctx, s.client, rawURL, s.waitForHost)
	if err != nil {
		return Result{LinkID: id, Err: fmt.Errorf("resolve %s: %w", rawURL, err)}
	}
	scrapeURL := resolved.URL

	if err := s.waitForHost(ctx, scrapeURL); err != nil {
		return Result{LinkID: id, Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scrapeURL, nil)
	if err != nil {
		return Result{LinkID: id, Err: fmt.Errorf("create request: %w", err)}
	}
	req.Header.Set("User-Agent", "TheDredger/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return Result{LinkID: id, Err: fmt.Errorf("fetch %s: %w", scrapeURL, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return Result{LinkID: id, Err: httpStatusError(resp)}
	}

	limited := io.LimitReader(resp.Body, 1<<20) // 1MB
	meta := ScrapeMetadata(limited)

	title := meta.Title
	if title == "" {
		title = rawURL
	}

	return Result{
		LinkID:      id,
		Title:       title,
		Description: meta.Description,
		Comments:    resolved.Comments,
	}
}

func (s *Service) waitForHost(ctx context.Context, rawURL string) error {
	if s.hostDelay <= 0 {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL for host pacing: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil
	}

	now := time.Now()
	scheduled := now

	s.hostMu.Lock()
	if last, ok := s.lastFetch[host]; ok {
		next := last.Add(s.hostDelay)
		if scheduled.Before(next) {
			scheduled = next
		}
	}
	s.lastFetch[host] = scheduled
	s.hostMu.Unlock()

	if wait := time.Until(scheduled); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func httpStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyText := strings.TrimSpace(string(body))

	detail := fmt.Sprintf("fetch returned status %d", resp.StatusCode)
	if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
		detail += fmt.Sprintf(" (retry-after: %s)", retryAfter)
	}
	if bodyText != "" {
		detail += fmt.Sprintf(": %s", bodyText)
	}
	return fmt.Errorf("%s", detail)
}
