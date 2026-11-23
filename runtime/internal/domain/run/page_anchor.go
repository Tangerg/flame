package run

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type PageAnchor struct {
	createdAt time.Time
	runID     identity.RunID
}

func NewPageAnchor(createdAt time.Time, runID identity.RunID) (PageAnchor, error) {
	if err := runID.Validate(); err != nil {
		return PageAnchor{}, fmt.Errorf("run: page anchor: %w", err)
	}
	return PageAnchor{createdAt: createdAt.UTC(), runID: runID}, nil
}

func (a PageAnchor) CreatedAt() time.Time  { return a.createdAt }
func (a PageAnchor) RunID() identity.RunID { return a.runID }
