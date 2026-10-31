package run

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// PageAnchor is the (creation time, Run identity) position a keyset page of
// Runs or of their interrupts ended at; the identity makes the order total,
// since two Runs can be created in the same nanosecond. A missing anchor is
// the first page, so no zero time or empty identity stands in for absence.
type PageAnchor struct {
	createdAt time.Time
	runID     string
}

func NewPageAnchor(createdAt time.Time, runID string) (PageAnchor, error) {
	if err := identity.ValidateRun(runID); err != nil {
		return PageAnchor{}, fmt.Errorf("run: page anchor: %w", err)
	}
	return PageAnchor{createdAt: createdAt.UTC(), runID: runID}, nil
}

func (a PageAnchor) CreatedAt() time.Time { return a.createdAt }
func (a PageAnchor) RunID() string        { return a.runID }
