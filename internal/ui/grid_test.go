package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/alexzajac/the-dredger/internal/model"
)

func TestGridApplySearchUsesSharedScorer(t *testing.T) {
	grid := GridModel{
		links: []model.Link{
			{ID: 1, URL: "https://example.com/title", Title: "Shader techniques"},
			{ID: 2, URL: "https://example.com/summary", Title: "Unrelated", Summary: "An introduction to shader programming."},
		},
		searchQuery: "shader",
		cursorX:     2,
		cursorY:     3,
		scrollY:     4,
	}

	grid.applySearch()

	if len(grid.filtered) != 2 {
		t.Fatalf("filtered links = %d, want 2", len(grid.filtered))
	}
	if grid.filtered[0].ID != 1 || grid.filtered[1].ID != 2 {
		t.Errorf("ranked links = [%d %d], want [1 2]", grid.filtered[0].ID, grid.filtered[1].ID)
	}
	if grid.cursorX != 0 || grid.cursorY != 0 || grid.scrollY != 0 {
		t.Errorf("search did not reset cursor: x=%d y=%d scroll=%d", grid.cursorX, grid.cursorY, grid.scrollY)
	}
}

func TestGridSearchRemainsActiveAfterEnter(t *testing.T) {
	grid := NewGridModel(nil, 120, 30)
	grid.links = []model.Link{
		{ID: 1, Title: "Unrelated"},
		{ID: 2, Title: "Notes", Summary: "Shader programming"},
	}
	grid, _ = grid.Update(tea.KeyPressMsg{Code: '/'})
	for _, r := range "shader" {
		grid, _ = grid.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	grid, _ = grid.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if grid.searching {
		t.Fatal("Enter did not finish editing the query")
	}
	if got := grid.activeLinks(); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("accepted search shows wrong links: %v", got)
	}
	if got := grid.selectedLink(); got == nil || got.ID != 2 {
		t.Fatalf("accepted search selects wrong link: %v", got)
	}
	grid, cmd := grid.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || grid.searchQuery != "" || len(grid.activeLinks()) != 2 {
		t.Fatal("Escape did not clear the accepted search in place")
	}
}
