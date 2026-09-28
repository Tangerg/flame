package persistence

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

type TrajectoryReader struct{ store *sqlite.TrajectoryStore }

func (r *TrajectoryReader) PageTrajectory(ctx context.Context, sessionID string, includeDescendants bool, anchor *sessions.TrajectoryPosition, limit int) ([]sessions.TrajectoryEntry, error) {
	var position *sqlite.TrajectoryPosition
	if anchor != nil {
		position = &sqlite.TrajectoryPosition{OccurredAt: anchor.OccurredAt, Kind: string(anchor.Kind), ID: anchor.ID}
	}
	rows, err := r.store.Page(ctx, sessionID, includeDescendants, position, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]sessions.TrajectoryEntry, len(rows))
	for index, row := range rows {
		entry := sessions.TrajectoryEntry{OccurredAt: row.OccurredAt, Run: row.Run, Item: row.Item}
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
