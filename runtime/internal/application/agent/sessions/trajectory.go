package sessions

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

type TrajectoryKind string

const (
	TrajectoryRun   TrajectoryKind = "run"
	TrajectoryModel TrajectoryKind = "model"
	TrajectoryItem  TrajectoryKind = "item"
)

type TrajectoryModelInvocation struct {
	RunID string
	runs.ModelInvocationCommit
}

type TrajectoryEntry struct {
	OccurredAt time.Time
	Run        *run.Run
	Model      *TrajectoryModelInvocation
	Item       *transcript.Item
}

func (e TrajectoryEntry) Position() TrajectoryPosition {
	switch {
	case e.Run != nil:
		return TrajectoryPosition{OccurredAt: e.OccurredAt.UnixNano(), Kind: TrajectoryRun, ID: e.Run.ID()}
	case e.Model != nil:
		return TrajectoryPosition{OccurredAt: e.OccurredAt.UnixNano(), Kind: TrajectoryModel, ID: e.Model.CallID}
	case e.Item != nil:
		return TrajectoryPosition{OccurredAt: e.OccurredAt.UnixNano(), Kind: TrajectoryItem, ID: e.Item.ID()}
	default:
		return TrajectoryPosition{}
	}
}

type TrajectoryPosition struct {
	OccurredAt int64
	Kind       TrajectoryKind
	ID         string
}

func (p TrajectoryPosition) validate() error {
	switch p.Kind {
	case TrajectoryRun:
		return resourceid.ValidateRun(p.ID)
	case TrajectoryModel:
		return runtimeidentity.ValidateEffect(p.ID)
	case TrajectoryItem:
		return resourceid.ValidateItem(p.ID)
	default:
		return errors.New("sessions: unknown trajectory kind")
	}
}

// QueryTrajectoryReader returns a bounded page hydrated from one database
// snapshot, including the Session existence check and descendant filter.
type QueryTrajectoryReader interface {
	PageTrajectory(context.Context, string, bool, *TrajectoryPosition, int) ([]TrajectoryEntry, error)
}

func (c *QueryCoordinator) ListTrajectoryPage(ctx context.Context, sessionID string, includeDescendants bool, cursor string, limit pagination.RequestedLimit) (pagination.Page[TrajectoryEntry], error) {
	const namespace = "session-trajectory"
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	filters := []string{sessionID, strconv.FormatBool(includeDescendants)}
	key, err := pagination.Decode(cursor, namespace, filters)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	anchor, err := trajectoryAnchor(key)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	size, err := limit.Resolve(100)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	rows, err := c.trajectory.PageTrajectory(ctx, sessionID, includeDescendants, anchor, size+1)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	return pagination.PageOf(rows, size, namespace, filters, func(entry TrajectoryEntry) []string {
		position := entry.Position()
		return []string{strconv.FormatInt(position.OccurredAt, 10), string(position.Kind), position.ID}
	})
}

func trajectoryAnchor(key []string) (*TrajectoryPosition, error) {
	if len(key) == 0 {
		return nil, nil
	}
	if len(key) != 3 {
		return nil, pagination.ErrInvalidCursor
	}
	occurredAt, err := strconv.ParseInt(key[0], 10, 64)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	position := TrajectoryPosition{OccurredAt: occurredAt, Kind: TrajectoryKind(key[1]), ID: key[2]}
	if err := position.validate(); err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	return &position, nil
}
