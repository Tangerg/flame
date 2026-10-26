package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

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
	Revision  uint64
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

// Validate reports whether an item is a coherent visible memory record: it needs
// an identity and content, a known scope, origin and status, timestamps, and an
// update no earlier than creation. A user-authored item is only ever active.
func (i Item) Validate() error {
	if i.Revision == 0 || i.Revision > exactint.Maximum {
		return fmt.Errorf("agent memory item requires a positive exact revision")
	}
	if i.ID == "" || strings.TrimSpace(i.Content) == "" {
		return fmt.Errorf("agent memory item requires identity and content")
	}
	if i.Scope != ScopeProject && i.Scope != ScopeUser {
		return fmt.Errorf("agent memory item has unknown scope %q", i.Scope)
	}
	if i.Origin != OriginAuto && i.Origin != OriginUser {
		return fmt.Errorf("agent memory item has unknown origin %q", i.Origin)
	}
	if i.Status != StatusActive && i.Status != StatusPending || i.Origin == OriginUser && i.Status != StatusActive {
		return fmt.Errorf("agent memory item has an invalid visible status %q", i.Status)
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() {
		return fmt.Errorf("agent memory item requires timestamps")
	}
	if i.UpdatedAt.Before(i.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", i.ID)
	}
	return nil
}
