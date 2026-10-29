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
	Scope     UpdateV2OutputScope
	Content   string
	Origin    UpdateV2OutputOrigin
	Status    UpdateV2OutputStatus
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
	if i.Scope != UpdateV2OutputScopeProject && i.Scope != UpdateV2OutputScopeUser {
		return fmt.Errorf("agent memory item has unknown scope %q", i.Scope)
	}
	if i.Origin != UpdateV2OutputOriginAuto && i.Origin != UpdateV2OutputOriginUser {
		return fmt.Errorf("agent memory item has unknown origin %q", i.Origin)
	}
	if i.Status != UpdateV2OutputStatusActive && i.Status != UpdateV2OutputStatusPending || i.Origin == UpdateV2OutputOriginUser && i.Status != UpdateV2OutputStatusActive {
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
