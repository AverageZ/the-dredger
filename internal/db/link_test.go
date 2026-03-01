package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alexzajac/the-dredger/internal/model"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndGetLinks(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{
		URL:         "https://example.com",
		Title:       "Example",
		Description: "An example site",
		Tags:        []string{"test", "example"},
		Status:      model.Unprocessed,
	}

	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert link: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	got := links[0]
	if got.URL != link.URL {
		t.Errorf("URL = %q, want %q", got.URL, link.URL)
	}
	if got.Title != link.Title {
		t.Errorf("Title = %q, want %q", got.Title, link.Title)
	}
	if got.Description != link.Description {
		t.Errorf("Description = %q, want %q", got.Description, link.Description)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "test" || got.Tags[1] != "example" {
		t.Errorf("Tags = %v, want [test example]", got.Tags)
	}
	if got.Status != model.Unprocessed {
		t.Errorf("Status = %v, want Unprocessed", got.Status)
	}
}

func TestInsertDuplicateURL(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{URL: "https://duplicate.com"}

	if _, err := InsertLink(db, link); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err := InsertLink(db, link)
	if err == nil {
		t.Fatal("expected error on duplicate URL, got nil")
	}
}

func TestUpdateLink(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{
		URL:   "https://update.com",
		Title: "Before",
	}
	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	link.ID = id
	link.Title = "After"
	link.Status = model.Saved
	if err := UpdateLink(db, link); err != nil {
		t.Fatalf("update: %v", err)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if links[0].Title != "After" {
		t.Errorf("Title = %q, want %q", links[0].Title, "After")
	}
	if links[0].Status != model.Saved {
		t.Errorf("Status = %v, want Saved", links[0].Status)
	}
}

func TestDeleteLink(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{URL: "https://delete.com"}
	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := DeleteLink(db, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("get links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected 0 links after delete, got %d", len(links))
	}
}

func TestGetLinksByStatus(t *testing.T) {
	db := setupTestDB(t)

	statuses := []model.Status{model.Unprocessed, model.Saved, model.Pruned}
	for i, s := range statuses {
		link := model.Link{
			URL:    "https://status.com/" + string(rune('a'+i)),
			Status: s,
		}
		id, err := InsertLink(db, link)
		if err != nil {
			t.Fatalf("insert link %d: %v", i, err)
		}
		// InsertLink always inserts with the provided status via the query,
		// but status is set in the INSERT; update to set non-zero statuses.
		if s != model.Unprocessed {
			link.ID = id
			if err := UpdateLink(db, link); err != nil {
				t.Fatalf("update link %d: %v", i, err)
			}
		}
	}

	saved, err := GetLinksByStatus(db, model.Saved)
	if err != nil {
		t.Fatalf("get links by status: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved link, got %d", len(saved))
	}
	if saved[0].Status != model.Saved {
		t.Errorf("Status = %v, want Saved", saved[0].Status)
	}
}

func TestCountLinksByStatus(t *testing.T) {
	db := setupTestDB(t)

	links := []model.Link{
		{URL: "https://count1.com"},
		{URL: "https://count2.com"},
		{URL: "https://count3.com"},
	}
	for i, l := range links {
		id, err := InsertLink(db, l)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		links[i].ID = id
	}

	// Update second link to Saved, third to Pruned.
	links[1].Status = model.Saved
	if err := UpdateLink(db, links[1]); err != nil {
		t.Fatalf("update: %v", err)
	}
	links[2].Status = model.Pruned
	if err := UpdateLink(db, links[2]); err != nil {
		t.Fatalf("update: %v", err)
	}

	stats, err := CountLinksByStatus(db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if stats.Unprocessed != 1 {
		t.Errorf("Unprocessed = %d, want 1", stats.Unprocessed)
	}
	if stats.Saved != 1 {
		t.Errorf("Saved = %d, want 1", stats.Saved)
	}
	if stats.Pruned != 1 {
		t.Errorf("Pruned = %d, want 1", stats.Pruned)
	}
	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}
}
