# The Dredger

A terminal UI for turning your chaotic pile of unsorted bookmarks into an organized, searchable idea mine. Built with Go and the [Charmbracelet](https://charm.sh/) stack.

## Prerequisites

- Go 1.25+

## Build & Run

```bash
go build -o dredger ./cmd/dredger
./dredger
```

Or run directly:

```bash
go run ./cmd/dredger
```

> **Note:** You must rebuild after pulling changes or editing code. A stale binary won't include new features.

## Importing Links

Feed it a file containing URLs (one per line, or mixed text — URLs are extracted automatically):

```bash
./dredger import ~/bookmarks.txt
```

Importing only writes URLs to SQLite; it does not crawl remote sites. Bulk dredging starts only when you press `r` in list mode, and it processes a bounded batch by default.

## Keybindings

### List Mode

| Key           | Action                                               |
| ------------- | ---------------------------------------------------- |
| `↑` / `↓`     | Navigate links                                       |
| `enter` / `f` | Enter focus mode                                     |
| `c`           | Browse collections by tag                            |
| `g`           | Open grid view                                       |
| `d`           | Re-dredge the selected link                          |
| `r`           | Dredge the next safe batch, including failed dredges |
| `/`           | Filter links                                         |
| `q`           | Quit                                                 |

### Focus Mode

Review bookmarks one-by-one:

| Key   | Action                                       |
| ----- | -------------------------------------------- |
| `h`   | Delete bookmark                              |
| `t`   | Tag                                          |
| `r`   | Read                                         |
| `d`   | Re-dredge with metadata, summaries, and tags |
| `z`   | Undo last delete                             |
| `esc` | Back to list                                 |

### Dredging States

When you press `d` on a link, dredging progresses through:

1. **Crawling** — the link is being fetched and data gathered
2. **Crunching** — contents are being summarized by an LLM
3. **Complete** — done, entry updated with metadata & summary
4. **Capsized** — failed (error message preserved)

## Data Storage

All data lives in a SQLite database at `~/.dredger/dredger.db`.

## Dredging Safety

The crawler is conservative by default:

- `DREDGER_DREDGE_BATCH_SIZE=50` limits each list-mode dredge batch
- `DREDGER_WORKERS=3` controls total crawl concurrency
- `DREDGER_HOST_DELAY=2s` spaces requests to the same host

## LLM Service

Dredger uses Ollama for AI summaries and tags by default:

```bash
./dredger --service=ollama
```

You can point dredging at LM Studio instead:

```bash
./dredger --service=lmstudio
```

The defaults are:

- `DREDGER_LLM_SERVICE=ollama`
- `DREDGER_OLLAMA_URL=http://localhost:11434`
- `DREDGER_OLLAMA_MODEL=gemma4:e4b`
- `DREDGER_LMSTUDIO_URL=http://127.0.0.1:1234`
- `DREDGER_LMSTUDIO_MODEL=google/gemma-4-26b-a4b`

For a very large import, start gently:

```bash
DREDGER_WORKERS=1 DREDGER_DREDGE_BATCH_SIZE=25 DREDGER_HOST_DELAY=5s ./dredger
```

## Maintenance Commands

```bash
# Show bookmark counts
./dredger stats

# Permanently remove legacy pruned links
./dredger clean

# Delete all links and start fresh (prompts for confirmation)
./dredger reset
```

## Development

### Setup (one-time)

```bash
make install-tools   # installs golangci-lint
make install-hooks   # activates pre-commit hook
```

### Makefile Targets

| Target       | Description                               |
| ------------ | ----------------------------------------- |
| `make`       | Default — runs fmt, lint, test, and build |
| `make build` | Compile binary to `./dredger`             |
| `make run`   | Build and run                             |
| `make test`  | Run all tests                             |
| `make lint`  | Run golangci-lint                         |
| `make fmt`   | Format all Go files                       |
| `make tidy`  | Run `go mod tidy`                         |
| `make clean` | Remove build artifacts                    |

### Pre-commit Hook

Running `make install-hooks` sets up a Git pre-commit hook that automatically runs `gofmt`, `go vet`, and a `go mod tidy` check on every commit, catching common issues before they reach CI.

### CI

GitHub Actions runs format check, vet, lint, test, and build on every push and PR to `main`.
