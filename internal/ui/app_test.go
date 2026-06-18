package ui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/alexzajac/the-dredger/internal/model"
)

func TestVisiblePageLinksForDredgeUsesCurrentPage(t *testing.T) {
	app := appWithListLinks([]model.Link{
		{ID: 1, URL: "https://example.com/1"},
		{ID: 2, URL: "https://example.com/2"},
		{ID: 3, URL: "https://example.com/3"},
		{ID: 4, URL: "https://example.com/4"},
		{ID: 5, URL: "https://example.com/5"},
	})
	app.list.Paginator.PerPage = 2
	app.list.Paginator.Page = 1

	got := app.visiblePageLinksForDredge()
	wantIDs := []int64{3, 4}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d links, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Fatalf("got link ID %d at index %d, want %d", got[i].ID, i, want)
		}
	}
}

func TestVisiblePageLinksForDredgeIncludesCompletedLinks(t *testing.T) {
	app := appWithListLinks([]model.Link{
		{ID: 1, URL: "https://example.com/1", Enriched: true, DredgeState: model.DredgeComplete},
		{ID: 2, URL: "https://example.com/2", Enriched: true, DredgeState: model.DredgeComplete},
	})
	app.list.Paginator.PerPage = 2

	got := app.visiblePageLinksForDredge()
	if len(got) != 2 {
		t.Fatalf("got %d links, want 2", len(got))
	}
	for _, l := range got {
		if !l.Enriched || l.DredgeState != model.DredgeComplete {
			t.Fatalf("link %+v was not the completed link from the visible page", l)
		}
	}
}

func TestMarkLinksDredgingUpdatesVisibleItems(t *testing.T) {
	app := appWithListLinks([]model.Link{
		{ID: 1, URL: "https://example.com/1", DredgeState: model.DredgeComplete},
		{ID: 2, URL: "https://example.com/2", DredgeState: model.DredgeCapsized, DredgeError: "old model failed"},
	})

	app.markLinksDredging([]model.Link{{ID: 2}})

	item, ok := app.list.Items()[1].(linkItem)
	if !ok {
		t.Fatal("list item was not a linkItem")
	}
	if item.link.DredgeState != model.DredgeCrawling {
		t.Fatalf("DredgeState = %v, want Crawling", item.link.DredgeState)
	}
	if item.link.DredgeError != "" {
		t.Fatalf("DredgeError = %q, want empty", item.link.DredgeError)
	}
}

func appWithListLinks(links []model.Link) App {
	items := make([]list.Item, len(links))
	for i, l := range links {
		items[i] = linkItem{link: l}
	}
	return App{
		list: list.New(items, list.NewDefaultDelegate(), 0, 0),
	}
}
