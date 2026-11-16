# Agent Rules

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Dredger is a Go CLI/TUI application for managing and organizing links/URLs. It is high-performance and "juicy". It uses Bubble Tea for the terminal UI, Lipgloss for styling, and SQLite (via modernc.org/sqlite) for persistence. Designed to transform a chaotic "digital graveyard" of unsorted bookmarks into an organized, actionable idea mine.

## Commands

```bash
# Build
go build -o dredger ./cmd/dredger

# Run
go run ./cmd/dredger

# Format
go fmt ./...

# Test
go test ./...

# Module management
go mod tidy
```

## Architecture

**Entry point:** `cmd/dredger/main.go` — opens SQLite DB at `~/.dredger/dredger.db`, initializes schema, launches TUI.

**Internal packages:**

- `internal/model/` — Domain types. `Link` struct with Status enum (Unprocessed=0, Saved=1, Pruned=2). Tags stored as `[]string` in Go, comma-separated in DB.
- `internal/db/` — Data access layer. `db.go` handles connection/schema (`Open` for write paths, `OpenReadOnly` for read paths), `link.go` has CRUD operations (Insert, Get, Update, Delete). Uses parameterized queries, single-connection WAL mode.
- `internal/ui/` — Bubble Tea TUI. `app.go` is the main model, `list.go` adapts Link to the list component, `grid.go` is the grid view, `styles.go` defines Lipgloss styles (accent: `#7D56F4`).
- `internal/dredge/` — Enrichment pipeline. `dredge.go` runs a worker pool, `scrape.go` fetches page metadata, `ollama.go`/`lmstudio.go` call the LLM for summaries/tags.
- `internal/ingest/` — Bulk URL import. `ingest.go` extracts URLs from text and batch-inserts via transaction.
- `internal/search/` — Relevance scoring. Ranks links in memory with field weights (tags > title > summary) and a coverage factor. Backs both `dredger search` and the TUI grid search; there should be exactly one search implementation.
- `internal/webexport/` — Static HTML dashboard emitted by `dredger web`. No server involved.
- `internal/config/` — Env-var configuration and validation.
- `internal/logging/` — File-based `slog` setup.
- `internal/urlutil/` — Shared URL helpers (`Domain`, `Path`).

**Data flow (TUI):** `main.go` → `db.Open` → `db.InitSchema()` → `ui.NewApp(db)` → Bubble Tea event loop

**Data flow (read-only CLI):** `main.go` → `db.OpenReadOnly` → `search.Search` → stdout. Read commands (`search`, `show`, `tags`, `stats`) skip `InitSchema` so they never take a write lock while the TUI is running.

**Agent-facing surface:** `dredger search|show|tags` with `--json` is a contract used by AI agents in other projects via `skills/dredger-bookmarks/SKILL.md`. Two rules follow: an unknown subcommand must exit non-zero rather than launching the TUI (Bubble Tea on a non-TTY hangs the caller), and a weak search must return its closest matches plus tag suggestions rather than empty output.

## Conventions

- Error wrapping with `fmt.Errorf("context: %w", err)`
- Defer for resource cleanup
- No external logging framework
- SQLite bundled in pure Go (no CGO required)
- Go 1.25.0

## Core Pillars

The pillars must be followed. Always.

1. Provides Enrichment: Automatically "dredges" raw URLs to fetch titles, metadata, and AI-generated summaries so you can see what’s inside without clicking
2. Enables Rapid Pruning: Uses a "Tinder-style" workflow—using quick keybindings (H to prune, L to keep)—enhanced with smooth animations and spring physics to make sorting feel like a game.
3. Promotes Discovery: Organizes the "keepers" into a searchable, categorized database (SQLite), allowing you to mine your old interests for new inspiration.
4. "Juicy": Leverages the Charmbracelet stack (Go) to provide a premium aesthetic experience with gradients, spinners, and responsive layouts that make utility software feel alive.
