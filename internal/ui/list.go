package ui

import (
	"github.com/alexzajac/the-dredger/internal/model"
)

// linkItem adapts model.Link to the bubbles list.DefaultItem interface.
type linkItem struct {
	link model.Link
}

func (i linkItem) Title() string {
	if i.link.Title == "" {
		return i.link.URL
	}
	return i.link.Title
}

func (i linkItem) Description() string {
	if i.link.Title == "" {
		return "Awaiting enrichment..."
	}
	if i.link.Description != "" {
		return i.link.Description
	}
	return i.link.URL
}

func (i linkItem) FilterValue() string { return i.link.Title + " " + i.link.URL }
