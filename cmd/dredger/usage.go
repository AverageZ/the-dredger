package main

const rootUsage = `dredger — dredge a graveyard of bookmarks into an idea mine.

Usage:
  dredger                       launch the TUI
  dredger <command> [flags]

Commands:
  search <query>   search bookmarks by title, tags, summary and URL
  show <id>        print one bookmark in full
  tags             list the tag vocabulary with usage counts
  add <url|text>   add links from arguments or stdin
  import <file|->  import links from a file or stdin
  stats            print bookmark counts
  web              export a static HTML dashboard
  clean            permanently delete pruned links
  reset            delete every link
  help             print this message

Global flags:
  --service <ollama|lmstudio>   LLM service used for dredging

Run "dredger <command> --help" for command-specific flags.
`

const searchUsage = `Usage: dredger search [flags] <query>

Ranks bookmarks against a free-text query. Tags weigh most, then the title, then
the AI summary. Matching every term of a query beats matching one term often, so
prefer several concept words over a long phrase.

Flags:
  --json              emit JSON (the stable contract; text output may change)
  --limit N           maximum results, 0 for all (default 10)
  --tag T             boost links carrying tag T (repeatable, comma-separated)
  --tag-strict        require every --tag instead of just boosting it
  --domain D          restrict results to host D, e.g. github.com
  --full              print complete summaries instead of snippets
  --include-pruned    also search links you have pruned

Examples:
  dredger search wave function collapse
  dredger search "llm agents" --limit 5 --json
  dredger search rendering --tag graphics --full
`

const showUsage = `Usage: dredger show [flags] <id>

Prints one bookmark in full, including its complete summary. Ids come from
dredger search.

Flags:
  --json    emit JSON instead of text

Example:
  dredger show 412
`

const tagsUsage = `Usage: dredger tags [flags]

Lists the tag vocabulary with usage counts, most used first. Useful before
searching: it shows the words your bookmarks actually use.

Flags:
  --json           emit JSON instead of text
  --min-count N    hide tags used fewer than N times (default 1)

Example:
  dredger tags --min-count 5
`
