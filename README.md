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

For new finds, add URLs directly without editing your seed file:

```bash
./dredger add https://example.com/article
./dredger add "Read this: https://example.com/article and https://example.com/other"
pbpaste | ./dredger import -
```

Importing only writes URLs to SQLite; it does not crawl remote sites. Bulk dredging starts only when you press `r` in list mode, and it processes a bounded batch by default.

## Searching From the Command Line

Install once so `dredger` is on your `PATH`:

```bash
make install
```

Then search from anywhere:

```bash
dredger search wave function collapse
dredger search "llm agents" --limit 5
dredger search rendering --tag graphics --full
dredger show 412
dredger tags --min-count 5
```

Results are ranked, not merely filtered. Tags carry the most weight (they are the
concepts the LLM extracted during dredging), then the title, then the AI summary.
Strong results must cover every searchable query term; otherwise the best partial
matches are labeled as suggestions. Prefer a few concept words over a long phrase.

When nothing matches well, dredger says so and offers related tags from your own
collection rather than printing an empty list:

```
$ dredger search quantum knitting
No strong matches for "quantum knitting" — closest 1:

#72      0.50  Zero-K
               [zero-k, cryogenics, physics, computing, hardware]
```

| Flag               | Applies to | Description                                        |
| ------------------ | ---------- | -------------------------------------------------- |
| `--json`           | all three  | Machine-readable output (the stable contract)       |
| `--limit N`        | `search`   | Cap results, `0` for all (default 10)               |
| `--tag T`          | `search`   | Boost links carrying tag `T` (repeatable)           |
| `--tag-strict`     | `search`   | Require every `--tag` instead of boosting it        |
| `--domain D`       | `search`   | Only links on host `D`                              |
| `--full`           | `search`   | Print complete summaries instead of snippets        |
| `--include-pruned` | `search`   | Also search links you have pruned                   |
| `--min-count N`    | `tags`     | Hide tags used fewer than `N` times                 |

`search`, `show`, `tags`, and `stats` open the database read-only, so they are
safe to run while the TUI is open in another terminal.

Grid search (`g`, then `/`) uses the same ranking, including summary and
description matches. Press Enter to keep the filter while navigating; Escape
clears it. The list filter and exported web dashboard retain their existing
search behavior.

`dredger --help` and `dredger search|show|tags --help` work before a database
exists. Unknown commands exit with status 2 instead of launching the TUI.

### Letting AI agents search your bookmarks

The `--json` output is designed for coding agents mining your bookmarks for
inspiration. To make Claude Code reach for it automatically in any project,
install the skill:

```bash
mkdir -p ~/.claude/skills/dredger-bookmarks
cp skills/dredger-bookmarks/SKILL.md ~/.claude/skills/dredger-bookmarks/
```

Being user-level, it applies to every project without per-project configuration.

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

# Export a standalone static dashboard you can open in a browser
./dredger web --out ./dredger-web
```

Run `dredger help` for the full command list, or `dredger <command> --help` for a
command's flags.

## Development

### Setup (one-time)

```bash
make install-tools   # installs golangci-lint
make install-hooks   # activates pre-commit hook
```

### Makefile Targets

| Target       | Description                               |
| ------------ | ----------------------------------------- |
| `make`         | Default — runs fmt, lint, test, and build |
| `make build`   | Compile binary to `./dredger`             |
| `make install` | Install `dredger` onto your `PATH`        |
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
