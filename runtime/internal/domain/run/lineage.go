package run

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ErrInvalidLineage reports a Run whose root/child identity is incomplete or
// self-referential. Relationships to other Runs and Items are aggregate
// invariants checked by the tree transaction; this value owns the frame-local
// shape that every layer can validate without I/O.
var ErrInvalidLineage = errors.New("run: invalid lineage")

// Lineage is the immutable identity edge set of a Run.
//
// A root has the zero value. A child carries all three edges: the parent Run is
// the direct topology edge, the root Run is the tree control/stream scope, and
// the spawning Item anchors the child to the parent tool-call Item. The edges are
// unexported so a half-built child cannot be constructed outside [NewLineage].
type Lineage struct {
	spawnedByItemID string
	parentRunID     string
	rootRunID       string
}

// NewLineage builds a lineage from its edges: all empty for a root, all present
// for a child. Whether a child names itself is a rule of the Run that carries it,
// checked by [Lineage.Validate].
func NewLineage(spawnedByItemID, parentRunID, rootRunID string) (Lineage, error) {
	lineage := Lineage{spawnedByItemID: spawnedByItemID, parentRunID: parentRunID, rootRunID: rootRunID}
	if err := lineage.validateShape(); err != nil {
		return Lineage{}, err
	}
	return lineage, nil
}

// SpawnedByItemID returns the parent tool-call Item of a child, or "" for a root.
func (l Lineage) SpawnedByItemID() string { return l.spawnedByItemID }

// ParentRunID returns the direct parent of a child, or "" for a root.
func (l Lineage) ParentRunID() string { return l.parentRunID }

// RootRunID returns the tree root of a child, or "" for a root.
func (l Lineage) RootRunID() string { return l.rootRunID }

// Validate reports whether l is a root or child shape that does not name runID.
func (l Lineage) Validate(runID string) error {
	if err := identity.ValidateRun(runID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLineage, err)
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
	if l.spawnedByItemID == "" || l.parentRunID == "" || l.rootRunID == "" {
		return fmt.Errorf("%w: a child requires spawnedByItemId, parentRunId, and rootRunId together", ErrInvalidLineage)
	}
	if err := identity.ValidateItem(l.spawnedByItemID); err != nil {
		return fmt.Errorf("%w: spawned-by %v", ErrInvalidLineage, err)
	}
	if err := identity.ValidateRun(l.parentRunID); err != nil {
		return fmt.Errorf("%w: parent %v", ErrInvalidLineage, err)
	}
	if err := identity.ValidateRun(l.rootRunID); err != nil {
		return fmt.Errorf("%w: root %v", ErrInvalidLineage, err)
	}
	return nil
}

// IsRoot reports whether l has the root shape.
func (l Lineage) IsRoot() bool {
	return l == Lineage{}
}

// IsChild reports whether l has the complete child shape.
func (l Lineage) IsChild() bool {
	return l.spawnedByItemID != "" && l.parentRunID != "" && l.rootRunID != ""
}

// TreeRootID returns the root Run that owns the tree. Callers validate l
// before relying on the result.
func (l Lineage) TreeRootID(runID string) string {
	if l.IsRoot() {
		return runID
	}
	return l.rootRunID
}

// MessageMark is the conversation watermark a Run on this lineage records when
// its tree ends at treeMark: the root records it, and a child records none.
func (l Lineage) MessageMark(treeMark int) MessageMark {
	if l.IsChild() {
		return UnknownMessageMark()
	}
	return MessageMarkAt(treeMark)
}
