package ui

import (
	"github.com/alexzajac/the-dredger/internal/dredge"
	"github.com/alexzajac/the-dredger/internal/model"
)

type LinksLoadedMsg struct {
	Links []model.Link
	Err   error
}

type DredgeResultMsg struct {
	Result dredge.Result
}

type DredgeDoneMsg struct {
	Err error
}

type EnterFocusModeMsg struct{}

type FocusExitMsg struct{}

type NextLinkLoadedMsg struct {
	Link *model.Link
	Err  error
}

type NextLinkPrefetchedMsg struct {
	Link *model.Link
}

type AnimTickMsg struct{}

type LinkActionedMsg struct {
	Link   model.Link
	Action string
}

// TriggerDredgeLinkMsg requests a manual dredge of a specific link.
type TriggerDredgeLinkMsg struct {
	LinkID int64
	URL    string
}

type GridLinksLoadedMsg struct {
	Links []model.Link
	Err   error
}

type GridExitMsg struct{}

type SerendipityResultMsg struct {
	Links []model.Link
	Err   error
}

// DredgeLinkResultMsg returns the result of a single-link dredge.
type DredgeLinkResultMsg struct {
	LinkID      int64
	State       model.DredgeState
	Title       string
	Description string
	Summary     string
	Tags        []string
	Error       string
}
