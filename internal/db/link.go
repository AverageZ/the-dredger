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

func GetLinks(db *sql.DB) ([]model.Link, error) {
	rows, err := db.Query(`SELECT id, url, title, description, tags, status, enriched, date_added FROM links ORDER BY date_added DESC`)
	if err != nil {
		return nil, fmt.Errorf("query links: %w", err)
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		var l model.Link
		var tags string
		var dateStr string
		var status int
		var enriched int
		if err := rows.Scan(&l.ID, &l.URL, &l.Title, &l.Description, &tags, &status, &enriched, &dateStr); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		l.Status = model.Status(status)
		l.Enriched = enriched != 0
		if tags != "" {
			l.Tags = strings.Split(tags, ",")
		}
		l.DateAdded, _ = time.Parse("2006-01-02 15:04:05", dateStr)
		links = append(links, l)
	}
	return links, rows.Err()
}

func UpdateLink(db *sql.DB, link model.Link) error {
	tags := strings.Join(link.Tags, ",")
	_, err := db.Exec(
		`UPDATE links SET url=?, title=?, description=?, tags=?, status=? WHERE id=?`,
		link.URL, link.Title, link.Description, tags, int(link.Status), link.ID,
	)
	if err != nil {
		return fmt.Errorf("update link: %w", err)
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
	rows, err := db.Query(`SELECT id, url, title, description, tags, status, enriched, date_added FROM links WHERE enriched = 0 ORDER BY date_added ASC`)
	if err != nil {
		return nil, fmt.Errorf("query unprocessed links: %w", err)
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		var l model.Link
		var tags string
		var dateStr string
		var status int
		var enriched int
		if err := rows.Scan(&l.ID, &l.URL, &l.Title, &l.Description, &tags, &status, &enriched, &dateStr); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		l.Status = model.Status(status)
		l.Enriched = enriched != 0
		if tags != "" {
			l.Tags = strings.Split(tags, ",")
		}
		l.DateAdded, _ = time.Parse("2006-01-02 15:04:05", dateStr)
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
