package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
	"github.com/alexzajac/the-dredger/internal/search"
)

// defaultSearchLimit keeps a casual search cheap for an agent to run: ten
// digest-format hits land around 700 tokens. Callers wanting more pass --limit.
const defaultSearchLimit = 10

// suggestedTagCount is how many related tags to offer when a search comes back
// weak. The tag vocabulary is long-tail, so showing the words the corpus
// actually uses is the fastest route to a better second query.
const suggestedTagCount = 8

// parseInterspersed parses flags that appear before, after, or between
// positional arguments, returning the positional ones.
//
// Go's flag package stops at the first non-flag argument, so a plain fs.Parse
// would fold `--limit 3` into the query of `dredger search shader --limit 3`.
// Trailing flags are how people — and agents — actually type commands, and
// silently searching for the literal text "--limit 3" produces wrong rankings
// with no error to notice.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for len(args) > 0 {
		if args[0] == "--" {
			return append(positional, args[1:]...), nil
		}
		if args[0] == "-" || !strings.HasPrefix(args[0], "-") {
			positional = append(positional, args[0])
			args = args[1:]
			continue
		}
		// Parse one flag and its value at a time so a terminator is never
		// consumed along with flags and then accidentally parsed again.
		n := 1
		name, _, hasValue := strings.Cut(strings.TrimLeft(args[0], "-"), "=")
		if f := fs.Lookup(name); f != nil && !hasValue {
			boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
			if (!ok || !boolean.IsBoolFlag()) && len(args) > 1 {
				n = 2
			}
		}
		if err := fs.Parse(args[:n]); err != nil {
			return nil, err
		}
		args = args[n:]
	}
	return positional, nil
}

// exitCodeForFlagErr maps a flag parse failure to an exit status. Asking for
// help succeeded at what it set out to do, so it exits 0; a malformed flag is a
// usage error and exits 2.
func exitCodeForFlagErr(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

// stringList collects a repeatable flag, e.g. --tag go --tag cli.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func runSearch(path string, args []string) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, searchUsage)
	}
	var tags stringList
	fs.Var(&tags, "tag", "boost links carrying this tag (repeatable)")
	asJSON := fs.Bool("json", false, "emit JSON instead of the human digest")
	limit := fs.Int("limit", defaultSearchLimit, "maximum results (0 for all)")
	tagStrict := fs.Bool("tag-strict", false, "require every --tag instead of just boosting")
	domain := fs.String("domain", "", "restrict results to this host")
	full := fs.Bool("full", false, "show complete summaries instead of snippets")
	includePruned := fs.Bool("include-pruned", false, "include pruned links")
	words, err := parseInterspersed(fs, args)
	if err != nil {
		os.Exit(exitCodeForFlagErr(err))
	}

	query := strings.Join(words, " ")
	if strings.TrimSpace(query) == "" && len(tags) == 0 {
		fmt.Fprint(os.Stderr, searchUsage)
		os.Exit(2)
	}
	database := openReadOnlyDatabase(path)
	defer func() { _ = database.Close() }()

	links, err := loadSearchCorpus(database, *includePruned)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading links: %v\n", err)
		os.Exit(1)
	}

	results := search.Search(links, search.Options{
		Query:         query,
		Tags:          tags,
		TagStrict:     *tagStrict,
		Domain:        *domain,
		Limit:         *limit,
		IncludePruned: *includePruned,
	})

	// Only spend the work on suggestions when the results are weak or absent —
	// that is the only time they help.
	var suggested []string
	if len(results) == 0 || results[0].Loose {
		suggested = search.SuggestTags(links, query, suggestedTagCount)
	}

	if *asJSON {
		enriched := 0
		for _, l := range links {
			if l.Enriched {
				enriched++
			}
		}
		if err := search.WriteJSON(os.Stdout, query, results, suggested, len(links), enriched); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := search.WriteDigest(os.Stdout, query, results, suggested, *full); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing search results: %v\n", err)
		os.Exit(1)
	}
}

func runShow(path string, args []string) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, showUsage)
	}
	asJSON := fs.Bool("json", false, "emit JSON instead of human-readable text")
	words, err := parseInterspersed(fs, args)
	if err != nil {
		os.Exit(exitCodeForFlagErr(err))
	}

	if len(words) != 1 {
		fmt.Fprint(os.Stderr, showUsage)
		os.Exit(2)
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(words[0], "#"), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid link id %q — expected a number.\n", words[0])
		os.Exit(2)
	}
	database := openReadOnlyDatabase(path)
	defer func() { _ = database.Close() }()

	link, err := db.GetLinkByID(database, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading link: %v\n", err)
		os.Exit(1)
	}
	if link == nil {
		fmt.Fprintf(os.Stderr, "No link with id %d.\n", id)
		os.Exit(1)
	}

	if *asJSON {
		if err := search.WriteLinkJSON(os.Stdout, *link); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := search.WriteLink(os.Stdout, *link); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing link: %v\n", err)
		os.Exit(1)
	}
}

func runTags(path string, args []string) {
	fs := flag.NewFlagSet("tags", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, tagsUsage)
	}
	asJSON := fs.Bool("json", false, "emit JSON instead of human-readable text")
	minCount := fs.Int("min-count", 1, "hide tags used fewer than this many times")
	words, err := parseInterspersed(fs, args)
	if err != nil {
		os.Exit(exitCodeForFlagErr(err))
	}
	if len(words) != 0 {
		fmt.Fprint(os.Stderr, tagsUsage)
		os.Exit(2)
	}
	database := openReadOnlyDatabase(path)
	defer func() { _ = database.Close() }()

	counts, err := db.GetTagCounts(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tags: %v\n", err)
		os.Exit(1)
	}

	type tagOut struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	out := make([]tagOut, 0, len(counts))
	for _, c := range counts {
		if c.Count >= *minCount {
			out = append(out, tagOut{Tag: c.Tag, Count: c.Count})
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(out) == 0 {
		fmt.Printf("No tags used at least %d time(s).\n", *minCount)
		return
	}
	for _, t := range out {
		fmt.Printf("%5d  %s\n", t.Count, t.Tag)
	}
	fmt.Printf("\n%d tags shown (of %d total).\n", len(out), len(counts))
}

// loadSearchCorpus fetches the links a search should consider. Pruned links are
// excluded by default at the SQL layer so the common case never loads them.
func loadSearchCorpus(database *sql.DB, includePruned bool) ([]model.Link, error) {
	if includePruned {
		return db.GetLinks(database)
	}
	return db.GetBookmarkLinks(database)
}
