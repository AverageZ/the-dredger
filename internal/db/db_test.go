package db

import (
	"database/sql"
	"testing"

	"github.com/alexzajac/the-dredger/internal/model"
)

func TestInitSchema_CreatesTable(t *testing.T) {
	db := setupTestDB(t)

	// Verify we can insert a row — table exists
	_, err := db.Exec(`INSERT INTO links (url) VALUES ('https://test.com')`)
	if err != nil {
		t.Fatalf("insert into links table: %v", err)
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	db := setupTestDB(t) // calls InitSchema once

	// Call again — should not error
	if err := InitSchema(db); err != nil {
		t.Fatalf("second InitSchema: %v", err)
	}
}

func TestInitSchema_MigrationColumns(t *testing.T) {
	db := setupTestDB(t)

	// Verify migrated columns exist by inserting with them
	_, err := db.Exec(
		`INSERT INTO links (url, enriched, dredge_state, dredge_error, summary)
		 VALUES ('https://migrate.com', 1, 2, 'err', 'sum')`,
	)
	if err != nil {
		t.Fatalf("insert with migrated columns: %v", err)
	}
}

func TestInitSchema_Indexes(t *testing.T) {
	db := setupTestDB(t)

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'idx_links_%'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()

	indexes := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		indexes[name] = true
	}

	expected := []string{"idx_links_status", "idx_links_enriched", "idx_links_dredge_state"}
	for _, idx := range expected {
		if !indexes[idx] {
			t.Errorf("missing index %q", idx)
		}
	}
}

func TestGetTagCounts(t *testing.T) {
	db := setupTestDB(t)

	// Insert bookmark links with overlapping tags.
	links := []model.Link{
		{URL: "https://a.com", Tags: []string{"go", "cli"}, Status: model.Saved},
		{URL: "https://b.com", Tags: []string{"go", "tui"}, Status: model.Saved},
		{URL: "https://c.com", Tags: []string{"go", "cli", "sqlite"}, Status: model.Saved},
		{URL: "https://d.com", Tags: []string{"python"}, Status: model.Unprocessed},
	}
	for _, l := range links {
		id, err := InsertLink(db, l)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		l.ID = id
		if err := UpdateLink(db, l); err != nil {
			t.Fatalf("update: %v", err)
		}
	}

	counts, err := GetTagCounts(db)
	if err != nil {
		t.Fatalf("GetTagCounts: %v", err)
	}

	// go=3, cli=2, python=1, sqlite=1, tui=1
	if len(counts) != 5 {
		t.Fatalf("expected 5 tags, got %d: %v", len(counts), counts)
	}
	if counts[0].Tag != "go" || counts[0].Count != 3 {
		t.Errorf("first tag = %v, want {go, 3}", counts[0])
	}
	if counts[1].Tag != "cli" || counts[1].Count != 2 {
		t.Errorf("second tag = %v, want {cli, 2}", counts[1])
	}
	// Count-1 tags are sorted alphabetically.
	if counts[2].Tag != "python" {
		t.Errorf("third tag = %q, want python", counts[2].Tag)
	}
	if counts[3].Tag != "sqlite" {
		t.Errorf("fourth tag = %q, want sqlite", counts[3].Tag)
	}
	if counts[4].Tag != "tui" {
		t.Errorf("fifth tag = %q, want tui", counts[4].Tag)
	}
}

func TestGetTagCounts_EmptyDB(t *testing.T) {
	db := setupTestDB(t)

	counts, err := GetTagCounts(db)
	if err != nil {
		t.Fatalf("GetTagCounts: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected 0 tags, got %d", len(counts))
	}
}

func TestDeletePrunedLinks(t *testing.T) {
	db := setupTestDB(t)

	links := []model.Link{
		{URL: "https://keep.com", Status: model.Saved},
		{URL: "https://prune1.com", Status: model.Pruned},
		{URL: "https://prune2.com", Status: model.Pruned},
		{URL: "https://pending.com", Status: model.Unprocessed},
	}
	for _, l := range links {
		id, err := InsertLink(db, l)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		l.ID = id
		if l.Status != model.Unprocessed {
			if err := UpdateLink(db, l); err != nil {
				t.Fatalf("update: %v", err)
			}
		}
	}

	removed, err := DeletePrunedLinks(db)
	if err != nil {
		t.Fatalf("DeletePrunedLinks: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	remaining, err := GetLinks(db)
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %d, want 2", len(remaining))
	}
}

func TestUpdateDredgeState(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{URL: "https://dredge.com"}
	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := UpdateDredgeState(db, id, model.DredgeCrawling, ""); err != nil {
		t.Fatalf("UpdateDredgeState: %v", err)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if links[0].DredgeState != model.DredgeCrawling {
		t.Errorf("DredgeState = %v, want Crawling", links[0].DredgeState)
	}
}

func TestUpdateDredgeState_SkipsPruned(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{URL: "https://pruned.com", Status: model.Pruned}
	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	link.ID = id
	if err := UpdateLink(db, link); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Should not update because status is Pruned
	if err := UpdateDredgeState(db, id, model.DredgeCrawling, ""); err != nil {
		t.Fatalf("UpdateDredgeState: %v", err)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if links[0].DredgeState != model.DredgeNone {
		t.Errorf("DredgeState = %v, want None (should skip pruned)", links[0].DredgeState)
	}
}

func TestUpdateDredgeResult(t *testing.T) {
	db := setupTestDB(t)

	link := model.Link{URL: "https://result.com"}
	id, err := InsertLink(db, link)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := UpdateDredgeResult(db, id, "Title", "Desc", "Summary", []string{"tag1", "tag2"}); err != nil {
		t.Fatalf("UpdateDredgeResult: %v", err)
	}

	links, err := GetLinks(db)
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	got := links[0]
	if got.Title != "Title" {
		t.Errorf("Title = %q, want Title", got.Title)
	}
	if got.Description != "Desc" {
		t.Errorf("Description = %q, want Desc", got.Description)
	}
	if got.Summary != "Summary" {
		t.Errorf("Summary = %q, want Summary", got.Summary)
	}
	if got.Enriched != true {
		t.Errorf("Enriched = %v, want true", got.Enriched)
	}
	if got.DredgeState != model.DredgeComplete {
		t.Errorf("DredgeState = %v, want Complete", got.DredgeState)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "tag1" {
		t.Errorf("Tags = %v, want [tag1 tag2]", got.Tags)
	}
}

func TestGetRandomSavedLinks(t *testing.T) {
	db := setupTestDB(t)

	// Insert 5 saved links with different first tags
	for i, tag := range []string{"go", "rust", "python", "java", "zig"} {
		l := model.Link{
			URL:    "https://random.com/" + tag,
			Tags:   []string{tag},
			Status: model.Saved,
		}
		id, err := InsertLink(db, l)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		l.ID = id
		if err := UpdateLink(db, l); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}

	result, err := GetRandomSavedLinks(db, 3)
	if err != nil {
		t.Fatalf("GetRandomSavedLinks: %v", err)
	}
	if len(result) > 3 {
		t.Errorf("got %d links, want at most 3", len(result))
	}
	if len(result) == 0 {
		t.Error("got 0 links, want at least 1")
	}

	// Verify distinct first tags
	seen := make(map[string]bool)
	for _, l := range result {
		tag := ""
		if len(l.Tags) > 0 {
			tag = l.Tags[0]
		}
		if seen[tag] {
			t.Errorf("duplicate first tag %q in random results", tag)
		}
		seen[tag] = true
	}
}

func TestGetNextUnprocessedExcluding(t *testing.T) {
	db := setupTestDB(t)

	l1 := model.Link{URL: "https://first.com"}
	id1, err := InsertLink(db, l1)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	l2 := model.Link{URL: "https://second.com"}
	_, err = InsertLink(db, l2)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := GetNextUnprocessedExcluding(db, id1)
	if err != nil {
		t.Fatalf("GetNextUnprocessedExcluding: %v", err)
	}
	if got == nil {
		t.Fatal("expected a link, got nil")
	}
	if got.URL != "https://second.com" {
		t.Errorf("URL = %q, want https://second.com", got.URL)
	}
}

func TestGetNextUnprocessedAfterFollowsInboxOrder(t *testing.T) {
	db := setupTestDB(t)

	oldest := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/oldest"}, "2024-01-01 10:00:00")
	middle := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/middle"}, "2024-01-02 10:00:00")
	newest := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/newest"}, "2024-01-03 10:00:00")

	got, err := GetNextUnprocessedAfter(db, newest)
	if err != nil {
		t.Fatalf("GetNextUnprocessedAfter newest: %v", err)
	}
	if got == nil || got.ID != middle {
		t.Fatalf("after newest got %#v, want middle ID %d", got, middle)
	}

	got, err = GetNextUnprocessedAfter(db, middle)
	if err != nil {
		t.Fatalf("GetNextUnprocessedAfter middle: %v", err)
	}
	if got == nil || got.ID != oldest {
		t.Fatalf("after middle got %#v, want oldest ID %d", got, oldest)
	}

	got, err = GetNextUnprocessedAfter(db, oldest)
	if err != nil {
		t.Fatalf("GetNextUnprocessedAfter oldest: %v", err)
	}
	if got != nil {
		t.Fatalf("after oldest got %#v, want nil", got)
	}
}

func TestGetPreviousUnprocessedBeforeFollowsInboxOrder(t *testing.T) {
	db := setupTestDB(t)

	oldest := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/oldest"}, "2024-01-01 10:00:00")
	middle := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/middle"}, "2024-01-02 10:00:00")
	newest := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/newest"}, "2024-01-03 10:00:00")

	got, err := GetPreviousUnprocessedBefore(db, oldest)
	if err != nil {
		t.Fatalf("GetPreviousUnprocessedBefore oldest: %v", err)
	}
	if got == nil || got.ID != middle {
		t.Fatalf("before oldest got %#v, want middle ID %d", got, middle)
	}

	got, err = GetPreviousUnprocessedBefore(db, middle)
	if err != nil {
		t.Fatalf("GetPreviousUnprocessedBefore middle: %v", err)
	}
	if got == nil || got.ID != newest {
		t.Fatalf("before middle got %#v, want newest ID %d", got, newest)
	}

	got, err = GetPreviousUnprocessedBefore(db, newest)
	if err != nil {
		t.Fatalf("GetPreviousUnprocessedBefore newest: %v", err)
	}
	if got != nil {
		t.Fatalf("before newest got %#v, want nil", got)
	}
}

func TestGetNextUnprocessedAfterUsesIDTieBreaker(t *testing.T) {
	db := setupTestDB(t)

	first := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/first"}, "2024-01-01 10:00:00")
	second := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/second"}, "2024-01-01 10:00:00")
	third := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/third"}, "2024-01-01 10:00:00")

	got, err := GetNextUnprocessedAfter(db, third)
	if err != nil {
		t.Fatalf("GetNextUnprocessedAfter third: %v", err)
	}
	if got == nil || got.ID != second {
		t.Fatalf("after third got %#v, want second ID %d", got, second)
	}

	got, err = GetNextUnprocessedAfter(db, second)
	if err != nil {
		t.Fatalf("GetNextUnprocessedAfter second: %v", err)
	}
	if got == nil || got.ID != first {
		t.Fatalf("after second got %#v, want first ID %d", got, first)
	}
}

func TestGetPreviousUnprocessedBeforeUsesIDTieBreaker(t *testing.T) {
	db := setupTestDB(t)

	first := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/first"}, "2024-01-01 10:00:00")
	second := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/second"}, "2024-01-01 10:00:00")
	third := insertDatedLink(t, db, model.Link{URL: "https://inbox.com/third"}, "2024-01-01 10:00:00")

	got, err := GetPreviousUnprocessedBefore(db, first)
	if err != nil {
		t.Fatalf("GetPreviousUnprocessedBefore first: %v", err)
	}
	if got == nil || got.ID != second {
		t.Fatalf("before first got %#v, want second ID %d", got, second)
	}

	got, err = GetPreviousUnprocessedBefore(db, second)
	if err != nil {
		t.Fatalf("GetPreviousUnprocessedBefore second: %v", err)
	}
	if got == nil || got.ID != third {
		t.Fatalf("before second got %#v, want third ID %d", got, third)
	}
}

func TestGetNextSavedAfterFollowsLibraryOrder(t *testing.T) {
	db := setupTestDB(t)

	oldest := insertDatedLink(t, db, model.Link{URL: "https://library.com/oldest", Status: model.Saved}, "2024-01-01 10:00:00")
	newest := insertDatedLink(t, db, model.Link{URL: "https://library.com/newest", Status: model.Saved}, "2024-01-02 10:00:00")

	got, err := GetNextSavedAfter(db, newest)
	if err != nil {
		t.Fatalf("GetNextSavedAfter newest: %v", err)
	}
	if got == nil || got.ID != oldest {
		t.Fatalf("after newest got %#v, want oldest ID %d", got, oldest)
	}
}

func TestGetPreviousSavedBeforeFollowsLibraryOrder(t *testing.T) {
	db := setupTestDB(t)

	oldest := insertDatedLink(t, db, model.Link{URL: "https://library.com/oldest", Status: model.Saved}, "2024-01-01 10:00:00")
	newest := insertDatedLink(t, db, model.Link{URL: "https://library.com/newest", Status: model.Saved}, "2024-01-02 10:00:00")

	got, err := GetPreviousSavedBefore(db, oldest)
	if err != nil {
		t.Fatalf("GetPreviousSavedBefore oldest: %v", err)
	}
	if got == nil || got.ID != newest {
		t.Fatalf("before oldest got %#v, want newest ID %d", got, newest)
	}
}

func insertDatedLink(t *testing.T, database *sql.DB, link model.Link, dateAdded string) int64 {
	t.Helper()
	id, err := InsertLink(database, link)
	if err != nil {
		t.Fatalf("insert dated link: %v", err)
	}
	if _, err := database.Exec(`UPDATE links SET status = ?, date_added = ? WHERE id = ?`, int(link.Status), dateAdded, id); err != nil {
		t.Fatalf("set dated link date: %v", err)
	}
	return id
}
