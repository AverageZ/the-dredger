package ingest

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alexzajac/the-dredger/internal/db"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single URL",
			input: "check out https://example.com for details",
			want:  []string{"https://example.com"},
		},
		{
			name:  "multiple URLs",
			input: "visit https://a.com and http://b.com today",
			want:  []string{"https://a.com", "http://b.com"},
		},
		{
			name:  "deduplicates",
			input: "https://dup.com and https://dup.com again",
			want:  []string{"https://dup.com"},
		},
		{
			name:  "trailing punctuation stripped",
			input: "See https://example.com. Also https://other.com, and https://third.com!",
			want:  []string{"https://example.com", "https://other.com", "https://third.com"},
		},
		{
			name:  "no URLs",
			input: "just some plain text with no links",
			want:  nil,
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "mixed protocols",
			input: "http://insecure.com https://secure.com",
			want:  []string{"http://insecure.com", "https://secure.com"},
		},
		{
			name:  "URL with path and query",
			input: "https://example.com/path?q=hello&page=2",
			want:  []string{"https://example.com/path?q=hello&page=2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractURLs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractURLs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestBulkInsert(t *testing.T) {
	t.Run("insert new URLs", func(t *testing.T) {
		database := setupTestDB(t)
		urls := []string{"https://a.com", "https://b.com", "https://c.com"}

		inserted, skipped, err := BulkInsert(database, urls)
		if err != nil {
			t.Fatalf("BulkInsert: %v", err)
		}
		if inserted != 3 {
			t.Errorf("inserted = %d, want 3", inserted)
		}
		if skipped != 0 {
			t.Errorf("skipped = %d, want 0", skipped)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		database := setupTestDB(t)

		inserted, skipped, err := BulkInsert(database, nil)
		if err != nil {
			t.Fatalf("BulkInsert: %v", err)
		}
		if inserted != 0 || skipped != 0 {
			t.Errorf("got inserted=%d, skipped=%d, want 0, 0", inserted, skipped)
		}
	})

	t.Run("duplicates in same batch", func(t *testing.T) {
		database := setupTestDB(t)
		// First insert
		_, _, err := BulkInsert(database, []string{"https://dup.com"})
		if err != nil {
			t.Fatalf("first BulkInsert: %v", err)
		}
		// Second insert with same URL
		inserted, skipped, err := BulkInsert(database, []string{"https://dup.com", "https://new.com"})
		if err != nil {
			t.Fatalf("second BulkInsert: %v", err)
		}
		if inserted != 1 {
			t.Errorf("inserted = %d, want 1", inserted)
		}
		if skipped != 1 {
			t.Errorf("skipped = %d, want 1", skipped)
		}
	})
}
