package ingest

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `)\]}]+`)

func ExtractURLs(text string) []string {
	matches := urlRe.FindAllString(text, -1)

	seen := make(map[string]struct{}, len(matches))
	var urls []string
	for _, u := range matches {
		u = strings.TrimRight(u, ".,;:!?")
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}
	return urls
}

func BulkInsert(db *sql.DB, urls []string) (inserted, skipped int, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO links (url) VALUES (?)
		ON CONFLICT(url) DO UPDATE SET date_added = CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, u := range urls {
		res, err := stmt.Exec(u)
		if err != nil {
			return inserted, skipped, fmt.Errorf("insert url %q: %w", u, err)
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			inserted++
		} else {
			skipped++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit transaction: %w", err)
	}
	return inserted, skipped, nil
}
