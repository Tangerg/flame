package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
)

type TrajectoryReader struct{ store *trajectoryStore }

func (r *TrajectoryReader) PageTrajectory(ctx context.Context, sessionID string, includeDescendants bool, anchor *sessions.TrajectoryPosition, limit int) ([]sessions.TrajectoryEntry, error) {
	var position *trajectoryPosition
	if anchor != nil {
		position = &trajectoryPosition{OccurredAt: anchor.OccurredAt, Kind: string(anchor.Kind), ID: anchor.ID}
	}
	rows, err := r.store.Page(ctx, sessionID, includeDescendants, position, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]sessions.TrajectoryEntry, len(rows))
	for index, row := range rows {
		entry := sessions.TrajectoryEntry{OccurredAt: row.OccurredAt, Run: row.Run, Item: row.Item, Endpoint: row.Endpoint}
		if model := row.Model; model != nil {
			invocation, err := modelInvocationFromRecord(*model)
			if err != nil {
				return nil, err
			}
			entry.Model = &sessions.TrajectoryModelInvocation{RunID: row.RunID, ModelInvocationCommit: invocation}
		}
		entries[index] = entry
	}
	return entries, nil
}
