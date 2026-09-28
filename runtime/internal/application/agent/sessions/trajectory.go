package sessions

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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

func (p TrajectoryPosition) compare(other TrajectoryPosition) int {
	return cmp.Or(cmp.Compare(p.OccurredAt, other.OccurredAt), cmp.Compare(p.Kind, other.Kind), cmp.Compare(p.ID, other.ID))
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
	if len(rows) > size+1 {
		return pagination.Page[TrajectoryEntry]{}, errors.New("sessions: trajectory reader exceeded page bound")
	}
	previous := anchor
	for _, entry := range rows {
		position := entry.Position()
		if err := entry.validateFor(sessionID, includeDescendants); err != nil {
			return pagination.Page[TrajectoryEntry]{}, err
		}
		if previous != nil && position.compare(*previous) >= 0 {
			return pagination.Page[TrajectoryEntry]{}, errors.New("sessions: trajectory reader returned unordered observations")
		}
		previous = &position
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

func (e TrajectoryEntry) validateFor(sessionID string, includeDescendants bool) error {
	count := 0
	var occurredAt time.Time
	if e.Run != nil {
		count++
		if e.Run.SessionID() != sessionID || (!includeDescendants && e.Run.Lineage().IsChild()) {
			return errors.New("sessions: trajectory Run escaped its scope")
		}
		occurredAt = e.Run.CreatedAt()
	}
	if e.Model != nil {
		count++
		if err := resourceid.ValidateRun(e.Model.RunID); err != nil {
			return fmt.Errorf("sessions: trajectory model Run: %w", err)
		}
		if err := e.Model.ModelInvocationCommit.Validate(); err != nil {
			return fmt.Errorf("sessions: trajectory model observation: %w", err)
		}
		occurredAt = e.Model.StartedAt
	}
	if e.Item != nil {
		count++
		if e.Item.SessionID() != sessionID {
			return errors.New("sessions: trajectory Item escaped its Session")
		}
		occurredAt = e.Item.OccurredAt()
	}
	if count != 1 || !e.OccurredAt.Equal(occurredAt) {
		return errors.New("sessions: invalid trajectory observation")
	}
	if err := e.Position().validate(); err != nil {
		return fmt.Errorf("sessions: trajectory identity: %w", err)
	}
	return nil
}
