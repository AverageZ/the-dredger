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

## Keybindings

### List Mode

| Key       | Action                                                     |
| --------- | ---------------------------------------------------------- |
| `↑` / `↓` | Navigate links                                             |
| `f`       | Enter focus mode                                           |
| `r`       | Dredge — enrich links with titles, metadata, and summaries |
| `/`       | Filter links                                               |
| `q`       | Quit                                                       |

### Focus Mode

Review links one-by-one, Tinder-style:

| Key       | Action                             |
| --------- | ---------------------------------- |
| `h`       | Prune (discard)                    |
| `l`       | Keep (save)                        |
| `s`       | Snooze (revisit in 7 days)         |
| `t`       | Tag                                |
| `j` / `k` | Scroll description                 |
| `z`       | Undo last action (3-second window) |
| `esc`     | Back to list                       |

## Data Storage

All data lives in a SQLite database at `~/.dredger/dredger.db`.

## Maintenance Commands

```bash
# Show link counts by status
./dredger stats

# Permanently remove all pruned links
./dredger clean

# Delete all links and start fresh (prompts for confirmation)
./dredger reset
```
