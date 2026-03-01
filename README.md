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

| Key       | Action                         |
| --------- | ------------------------------ |
| `↑` / `↓` | Navigate links                 |
| `f`       | Enter focus mode               |
| `b`       | Switch to saved bookmarks view |
| `/`       | Filter links                   |
| `q`       | Quit                           |

### Focus Mode — Pending Bookmarks

Review pending links one-by-one, Tinder-style:

| Key   | Action                |
| ----- | --------------------- |
| `h`   | Prune (soft delete)   |
| `l`   | Keep (move to saved)  |
| `s`   | Snooze (stay pending) |
| `z`   | Undo last action      |
| `esc` | Back to list          |

### Focus Mode — Saved Bookmarks

Manage saved links with tagging, reading, and enrichment:

| Key   | Action                                        |
| ----- | --------------------------------------------- |
| `h`   | Prune (move back to pending)                  |
| `t`   | Tag                                           |
| `r`   | Read                                          |
| `d`   | Dredge (LLM enrich with metadata & summaries) |
| `z`   | Undo last action                              |
| `esc` | Back to list                                  |

### Dredging States

When you press `d` on a saved bookmark, dredging progresses through:

1. **Crawling** — the link is being fetched and data gathered
2. **Crunching** — contents are being summarized by an LLM
3. **Complete** — done, entry updated with metadata & summary
4. **Capsized** — failed (error message preserved)

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
