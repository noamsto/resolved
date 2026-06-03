package model

import (
	"fmt"
	"time"
)

type RefKind int

const (
	KindURL RefKind = iota
	KindShortForm
	KindBare
)

func (k RefKind) String() string {
	switch k {
	case KindURL:
		return "url"
	case KindShortForm:
		return "shortform"
	case KindBare:
		return "bare"
	}
	return "unknown"
}

type RefType int

const (
	TypeIssue RefType = iota
	TypePR
)

func (t RefType) String() string {
	if t == TypePR {
		return "pr"
	}
	return "issue"
}

type Confidence int

const (
	ConfHigh Confidence = iota
	ConfLow
)

func (c Confidence) String() string {
	if c == ConfLow {
		return "low"
	}
	return "high"
}

type Tier int

const (
	TierOpen Tier = iota
	TierClosed
	TierStale
	TierGone
	TierUnknown
)

func (t Tier) String() string {
	switch t {
	case TierOpen:
		return "open"
	case TierClosed:
		return "closed"
	case TierStale:
		return "stale"
	case TierGone:
		return "gone"
	}
	return "unknown"
}

// Reference is a single detected GitHub reference in a comment.
type Reference struct {
	File       string
	Line, Col  int
	Raw        string
	Kind       RefKind
	Owner      string
	Repo       string
	Number     int
	Type       RefType
	Keyword    string // "" if no stale keyword nearby
	Confidence Confidence
}

// Key is the cache/dedupe key, e.g. "owner/repo#123".
func (r Reference) Key() string {
	return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
}

// Status is the resolved state of a referenced issue/PR.
type Status struct {
	State     string // open | closed | merged | gone | unknown
	Title     string
	UpdatedAt time.Time
}

// Finding pairs a Reference with its Status and computed Tier.
type Finding struct {
	Reference
	Status
	Tier Tier
}
