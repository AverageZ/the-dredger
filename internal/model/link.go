package model

import "time"

type Status int

const (
	Unprocessed Status = iota
	Saved
	Pruned
)

type DredgeState int

const (
	DredgeNone DredgeState = iota
	DredgeCrawling
	DredgeCrunching
	DredgeComplete
	DredgeCapsized
)

type Link struct {
	ID          int64
	URL         string
	Title       string
	Description string
	Summary     string
	Tags        []string
	Status      Status
	Enriched    bool
	DredgeState DredgeState
	DredgeError string
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

func (s Status) Color() string {
	switch s {
	case Saved:
		return "#04B575"
	case Pruned:
		return "#FF4040"
	default:
		return "#7D56F4"
	}
}

func (d DredgeState) String() string {
	switch d {
	case DredgeCrawling:
		return "Crawling..."
	case DredgeCrunching:
		return "Crunching..."
	case DredgeComplete:
		return "Complete"
	case DredgeCapsized:
		return "Capsized"
	default:
		return ""
	}
}
