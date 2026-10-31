package memory

import (
	"fmt"
	"strings"
	"time"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

type Item struct {
	ID        string
	Revision  uint64
	Scope     memorycontract.ItemSnapshotScope
	Content   string
	Origin    memorycontract.ItemSnapshotOrigin
	Status    memorycontract.ItemSnapshotStatus
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
	if i.Scope != memorycontract.ItemSnapshotScopeProject && i.Scope != memorycontract.ItemSnapshotScopeUser {
		return fmt.Errorf("agent memory item has unknown scope %q", i.Scope)
	}
	if i.Origin != memorycontract.ItemSnapshotOriginAuto && i.Origin != memorycontract.ItemSnapshotOriginUser {
		return fmt.Errorf("agent memory item has unknown origin %q", i.Origin)
	}
	if i.Status != memorycontract.ItemSnapshotStatusActive && i.Status != memorycontract.ItemSnapshotStatusPending || i.Origin == memorycontract.ItemSnapshotOriginUser && i.Status != memorycontract.ItemSnapshotStatusActive {
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
