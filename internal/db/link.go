package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/alexzajac/the-dredger/internal/model"
)

func InsertLink(db *sql.DB, link model.Link) (int64, error) {
	tags := strings.Join(link.Tags, ",")
	res, err := db.Exec(
		`INSERT INTO links (url, title, description, tags, status) VALUES (?, ?, ?, ?, ?)`,
		link.URL, link.Title, link.Description, tags, int(link.Status),
	)
	if err != nil {
		return 0, fmt.Errorf("insert link: %w", err)
	}
	return res.LastInsertId()
}

const linkSelectCols = `id, url, title, description, tags, status, enriched, date_added, dredge_state, dredge_error, summary`

func scanLink(scanner interface{ Scan(...any) error }) (model.Link, error) {
	var l model.Link
	var tags, dateStr, dredgeError, summary string
	var status, enriched, dredgeState int
	if err := scanner.Scan(&l.ID, &l.URL, &l.Title, &l.Description, &tags, &status, &enriched, &dateStr, &dredgeState, &dredgeError, &summary); err != nil {
		return l, err
	}
	l.Status = model.Status(status)
	l.Enriched = enriched != 0
	l.DredgeState = model.DredgeState(dredgeState)
	l.DredgeError = dredgeError
	l.Summary = summary
	if tags != "" {
		l.Tags = strings.Split(tags, ",")
	}
	l.DateAdded = parseDateStr(dateStr)
	return l, nil
}

// parseDateStr tries multiple time formats and falls back to time.Now().
func parseDateStr(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05.000000",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func GetLinks(db *sql.DB) ([]model.Link, error) {
	rows, err := db.Query(`SELECT ` + linkSelectCols + ` FROM links ORDER BY date_added DESC`)
	if err != nil {
		return nil, fmt.Errorf("query links: %w", err)
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

func GetLinksByStatus(db *sql.DB, status model.Status) ([]model.Link, error) {
	rows, err := db.Query(`SELECT `+linkSelectCols+` FROM links WHERE status = ? ORDER BY date_added DESC`, int(status))
	if err != nil {
		return nil, fmt.Errorf("query links by status: %w", err)
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

func UpdateLink(db *sql.DB, link model.Link) error {
	tags := strings.Join(link.Tags, ",")
	_, err := db.Exec(
		`UPDATE links SET url=?, title=?, description=?, tags=?, status=?, dredge_state=?, dredge_error=?, summary=? WHERE id=?`,
		link.URL, link.Title, link.Description, tags, int(link.Status), int(link.DredgeState), link.DredgeError, link.Summary, link.ID,
	)
	if err != nil {
		return fmt.Errorf("update link: %w", err)
	}
	return nil
}

// RestoreLink fully restores a link to a previous snapshot (used by undo).
func RestoreLink(db *sql.DB, link model.Link) error {
	tags := strings.Join(link.Tags, ",")
	_, err := db.Exec(
		`UPDATE links SET url=?, title=?, description=?, tags=?, status=?, date_added=?, dredge_state=?, dredge_error=?, summary=? WHERE id=?`,
		link.URL, link.Title, link.Description, tags, int(link.Status),
		link.DateAdded.Format("2006-01-02 15:04:05"),
		int(link.DredgeState), link.DredgeError, link.Summary, link.ID,
	)
	if err != nil {
		return fmt.Errorf("restore link: %w", err)
	}
	return nil
}

func DeleteLink(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM links WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}
	return nil
}

func GetUnprocessedLinks(db *sql.DB) ([]model.Link, error) {
	rows, err := db.Query(`SELECT ` + linkSelectCols + ` FROM links WHERE enriched = 0 ORDER BY date_added ASC`)
	if err != nil {
		return nil, fmt.Errorf("query unprocessed links: %w", err)
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

func UpdateLinkMeta(db *sql.DB, id int64, title, description string) error {
	_, err := db.Exec(`UPDATE links SET title=?, description=?, enriched=1 WHERE id=?`, title, description, id)
	if err != nil {
		return fmt.Errorf("update link meta: %w", err)
	}
	return nil
}

func GetNextUnprocessed(db *sql.DB) (*model.Link, error) {
	row := db.QueryRow(
		`SELECT ` + linkSelectCols + `
		 FROM links WHERE status = 0 AND date_added <= datetime('now')
		 ORDER BY date_added ASC LIMIT 1`,
	)
	l, err := scanLink(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get next unprocessed: %w", err)
	}
	return &l, nil
}

func GetNextSaved(db *sql.DB) (*model.Link, error) {
	row := db.QueryRow(
		`SELECT ` + linkSelectCols + `
		 FROM links WHERE status = 1
		 ORDER BY date_added DESC LIMIT 1`,
	)
	l, err := scanLink(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get next saved: %w", err)
	}
	return &l, nil
}

func SnoozeLink(db *sql.DB, id int64, until time.Time) error {
	_, err := db.Exec(`UPDATE links SET date_added = ? WHERE id = ?`,
		until.Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return fmt.Errorf("snooze link: %w", err)
	}
	return nil
}

func ResetLinkStatus(db *sql.DB, id int64, originalDateAdded time.Time) error {
	_, err := db.Exec(`UPDATE links SET status = 0, date_added = ? WHERE id = ?`,
		originalDateAdded.Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return fmt.Errorf("reset link status: %w", err)
	}
	return nil
}

// UpdateDredgeState sets the dredge state and optional error for a link.
// It re-checks the link status before writing to avoid overwriting a pruned link.
func UpdateDredgeState(db *sql.DB, id int64, state model.DredgeState, dredgeErr string) error {
	_, err := db.Exec(
		`UPDATE links SET dredge_state=?, dredge_error=? WHERE id=? AND status != ?`,
		int(state), dredgeErr, id, int(model.Pruned),
	)
	if err != nil {
		return fmt.Errorf("update dredge state: %w", err)
	}
	return nil
}

// UpdateDredgeResult sets the dredge state to complete and stores the fetched metadata.
// Skips if the link has been pruned (race condition guard).
func UpdateDredgeResult(db *sql.DB, id int64, title, description, summary string, tags []string) error {
	tagStr := strings.Join(tags, ",")
	_, err := db.Exec(
		`UPDATE links SET title=?, description=?, summary=?, tags=?, enriched=1, dredge_state=? WHERE id=? AND status != ?`,
		title, description, summary, tagStr, int(model.DredgeComplete), id, int(model.Pruned),
	)
	if err != nil {
		return fmt.Errorf("update dredge result: %w", err)
	}
	return nil
}

// LinkStats holds per-status link counts.
type LinkStats struct {
	Unprocessed int
	Saved       int
	Pruned      int
	Total       int
}

func CountLinksByStatus(db *sql.DB) (LinkStats, error) {
	rows, err := db.Query(`SELECT status, COUNT(*) FROM links GROUP BY status`)
	if err != nil {
		return LinkStats{}, fmt.Errorf("count links by status: %w", err)
	}
	defer rows.Close()

	var stats LinkStats
	for rows.Next() {
		var status, count int
		if err := rows.Scan(&status, &count); err != nil {
			return LinkStats{}, fmt.Errorf("scan link count: %w", err)
		}
		switch model.Status(status) {
		case model.Unprocessed:
			stats.Unprocessed = count
		case model.Saved:
			stats.Saved = count
		case model.Pruned:
			stats.Pruned = count
		}
		stats.Total += count
	}
	return stats, rows.Err()
}

func GetRandomSavedLinks(database *sql.DB, count int) ([]model.Link, error) {
	rows, err := database.Query(`SELECT `+linkSelectCols+` FROM links WHERE status = ? ORDER BY RANDOM()`, int(model.Saved))
	if err != nil {
		return nil, fmt.Errorf("query random saved links: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var result []model.Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		firstTag := ""
		if len(l.Tags) > 0 {
			firstTag = l.Tags[0]
		}
		if seen[firstTag] {
			continue
		}
		seen[firstTag] = true
		result = append(result, l)
		if len(result) >= count {
			break
		}
	}
	return result, rows.Err()
}

func DeletePrunedLinks(db *sql.DB) (int64, error) {
	res, err := db.Exec(`DELETE FROM links WHERE status = ?`, int(model.Pruned))
	if err != nil {
		return 0, fmt.Errorf("delete pruned links: %w", err)
	}
	return res.RowsAffected()
}

func DeleteAllLinks(db *sql.DB) (int64, error) {
	res, err := db.Exec(`DELETE FROM links`)
	if err != nil {
		return 0, fmt.Errorf("delete all links: %w", err)
	}
	return res.RowsAffected()
}
