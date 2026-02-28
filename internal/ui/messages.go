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
