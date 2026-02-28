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

# Test (no tests yet)
go test ./...

# Module management
go mod tidy
```

## Architecture

**Entry point:** `cmd/dredger/main.go` — opens SQLite DB at `~/.dredger/dredger.db`, initializes schema, launches TUI.

**Three internal packages:**

- `internal/model/` — Domain types. `Link` struct with Status enum (Unprocessed=0, Saved=1, Pruned=2). Tags stored as `[]string` in Go, comma-separated in DB.
- `internal/db/` — Data access layer. `db.go` handles connection/schema, `link.go` has CRUD operations (Insert, Get, Update, Delete). Uses parameterized queries, single-connection WAL mode.
- `internal/ui/` — Bubble Tea TUI. `app.go` is the main model, `list.go` adapts Link to the list component, `styles.go` defines Lipgloss styles (accent: `#7D56F4`).

**Data flow:** `main.go` → `db.InitSchema()` → `ui.NewApp(db)` → Bubble Tea event loop

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
