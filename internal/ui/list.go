package ui

import (
	"fmt"

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
	switch i.link.DredgeState {
	case model.DredgeCrawling, model.DredgeCrunching:
		return i.link.DredgeState.String()
	case model.DredgeCapsized:
		if i.link.DredgeError != "" {
			return fmt.Sprintf("Dredge failed: %s", i.link.DredgeError)
		}
		return "Dredge failed. Press d to retry."
	}
	if i.link.Title == "" {
		return "Awaiting enrichment..."
	}
	if i.link.Description != "" {
		return i.link.Description
	}
	return i.link.URL
}

func (i linkItem) FilterValue() string { return i.link.Title + " " + i.link.URL }
