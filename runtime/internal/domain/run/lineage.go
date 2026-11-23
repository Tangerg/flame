package run

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var ErrInvalidLineage = errors.New("run: invalid lineage")

type Lineage struct {
	spawnedByItemID identity.ItemID
	parentRunID     identity.RunID
	rootRunID       identity.RunID
}

func NewLineage(spawnedByItemID identity.ItemID, parentRunID, rootRunID identity.RunID) (Lineage, error) {
	lineage := Lineage{spawnedByItemID: spawnedByItemID, parentRunID: parentRunID, rootRunID: rootRunID}
	if err := lineage.validateShape(); err != nil {
		return Lineage{}, err
	}
	return lineage, nil
}

func (l Lineage) SpawnedByItemID() identity.ItemID { return l.spawnedByItemID }

func (l Lineage) ParentRunID() identity.RunID { return l.parentRunID }

func (l Lineage) RootRunID() identity.RunID { return l.rootRunID }

func (l Lineage) Validate(runID identity.RunID) error {
	if err := runID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidLineage, err)
	}
	if err := l.validateShape(); err != nil {
		return err
	}
	switch {
	case l.IsRoot():
		return nil
	case l.parentRunID == runID:
		return fmt.Errorf("%w: child run %q is its own parent", ErrInvalidLineage, runID)
	case l.rootRunID == runID:
		return fmt.Errorf("%w: child run %q is its own root", ErrInvalidLineage, runID)
	default:
		return nil
	}
}

func (l Lineage) validateShape() error {
	if l.IsRoot() {
		return nil
	}
	if l.spawnedByItemID.IsZero() || l.parentRunID.IsZero() || l.rootRunID.IsZero() {
		return fmt.Errorf("%w: a child requires spawnedByItemId, parentRunId, and rootRunId together", ErrInvalidLineage)
	}
	if err := l.spawnedByItemID.Validate(); err != nil {
		return fmt.Errorf("%w: spawned-by %w", ErrInvalidLineage, err)
	}
	if err := l.parentRunID.Validate(); err != nil {
		return fmt.Errorf("%w: parent %w", ErrInvalidLineage, err)
	}
	if err := l.rootRunID.Validate(); err != nil {
		return fmt.Errorf("%w: root %w", ErrInvalidLineage, err)
	}
	return nil
}

func (l Lineage) IsRoot() bool {
	return l == Lineage{}
}

func (l Lineage) IsChild() bool {
	return !l.spawnedByItemID.IsZero() && !l.parentRunID.IsZero() && !l.rootRunID.IsZero()
}

func (l Lineage) TreeRootID(runID identity.RunID) identity.RunID {
	if l.IsRoot() {
		return runID
	}
	return l.rootRunID
}

func (l Lineage) MessageMark(treeMark int) MessageMark {
	if l.IsChild() {
		return UnknownMessageMark()
	}
	return MessageMarkAt(treeMark)
}
