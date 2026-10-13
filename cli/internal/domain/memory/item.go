package memory

import "time"

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

type Origin string

const (
	OriginAuto Origin = "auto"
	OriginUser Origin = "user"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusPending Status = "pending"
)

// Item retains only the publisher fields used by terminal presentation.
type Item struct {
	ID        string
	Scope     Scope
	Content   string
	Origin    Origin
	Status    Status
	Pinned    bool
	SessionID string
	Day       string
	CreatedAt time.Time
	UpdatedAt time.Time
}
