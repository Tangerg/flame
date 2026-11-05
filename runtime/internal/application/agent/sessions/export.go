package sessions

import (
	"context"
	"fmt"
)

type ToolResultExport uint8

const (
	ReferencedToolResults ToolResultExport = iota
	ExpandedToolResults
)

// ExportSession keeps admission, hydration and terminal validation on the same
// captured state. A presentation cannot rebuild or validate a competing archive.
func (c *Coordinator) ExportSession(ctx context.Context, sessionID string, results ToolResultExport) (PortableSnapshot, error) {
	if results != ReferencedToolResults && results != ExpandedToolResults {
		return PortableSnapshot{}, fmt.Errorf("sessions: unsupported tool-result export %d", results)
	}
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return PortableSnapshot{}, err
	}
	defer admission.Release()

	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return PortableSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return PortableSnapshot{}, fmt.Errorf("sessions: validate export snapshot: %w", err)
	}
	if results == ExpandedToolResults {
		snapshot.Items, err = snapshot.HydratedItems()
		if err != nil {
			return PortableSnapshot{}, fmt.Errorf("sessions: expand exported tool results: %w", err)
		}
	}
	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		return PortableSnapshot{}, fmt.Errorf("sessions: prepare portable snapshot: %w", err)
	}
	return portable, nil
}
