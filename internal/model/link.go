package model

import "time"

type Status int

const (
	Unprocessed Status = iota
	Saved
	Pruned
)

type Link struct {
	ID          int64
	URL         string
	Title       string
	Description string
	Tags        []string
	Status      Status
	Enriched    bool
	DateAdded   time.Time
}

func (s Status) String() string {
	switch s {
	case Saved:
		return "saved"
	case Pruned:
		return "pruned"
	default:
		return "unprocessed"
	}
}
