package run

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
)

var ErrSessionBusy = errors.New("run: session busy")

type Draft struct {
	RunID             identity.RunID
	SessionID         identity.SessionID
	SpawnedByItemID   identity.ItemID
	ParentRunID       identity.RunID
	RootRunID         identity.RunID
	SegmentID         identity.SegmentID
	ModelSelection    flamemodel.Selection
	GoalIncarnationID goalref.IncarnationID
	// Admission is its only writer, which keeps it immutable for the Run's life by
	// construction.
	Capabilities Capabilities
	CreatedAt    time.Time
}

func (d Draft) Validate() error {
	if err := d.RunID.Validate(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := d.SessionID.Validate(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if d.SegmentID.IsZero() {
		return errors.New("run: opening Segment is required")
	}
	switch {
	case d.CreatedAt.IsZero():
		return errors.New("run: admission time is required")
	}
	lineage := d.Lineage()
	if err := lineage.Validate(d.RunID); err != nil {
		return err
	}
	if err := d.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := d.Capabilities.Validate(); err != nil {
		return err
	}
	if lineage.IsChild() && !d.GoalIncarnationID.IsZero() {
		return errors.New("run: child carries a root Goal incarnation")
	}
	if lineage.IsChild() && !d.Capabilities.IsEmpty() {
		return errors.New("run: child carries capabilities its root owns")
	}
	return nil
}

func (d Draft) Lineage() Lineage {
	return Lineage{
		spawnedByItemID: d.SpawnedByItemID,
		parentRunID:     d.ParentRunID,
		rootRunID:       d.RootRunID,
	}
}
