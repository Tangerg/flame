package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

type Item struct {
	ID        string
	Revision  uint64
	Scope     ItemSnapshotScope
	Content   string
	Origin    ItemSnapshotOrigin
	Status    ItemSnapshotStatus
	Pinned    bool
	SessionID string
	Day       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (i Item) Validate() error {
	if i.Revision == 0 || i.Revision > exactint.Maximum {
		return fmt.Errorf("agent memory item requires a positive exact revision")
	}
	if i.ID == "" || strings.TrimSpace(i.Content) == "" {
		return fmt.Errorf("agent memory item requires identity and content")
	}
	if i.Scope != ItemSnapshotScopeProject && i.Scope != ItemSnapshotScopeUser {
		return fmt.Errorf("agent memory item has unknown scope %q", i.Scope)
	}
	if i.Origin != ItemSnapshotOriginAuto && i.Origin != ItemSnapshotOriginUser {
		return fmt.Errorf("agent memory item has unknown origin %q", i.Origin)
	}
	if i.Status != ItemSnapshotStatusActive && i.Status != ItemSnapshotStatusPending || i.Origin == ItemSnapshotOriginUser && i.Status != ItemSnapshotStatusActive {
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
