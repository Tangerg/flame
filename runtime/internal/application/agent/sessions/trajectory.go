package sessions

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// TrajectoryKind names the timeline fact an entry projects. Its spelling is the
// durable ordering and cursor vocabulary.
type TrajectoryKind string

const (
	TrajectoryKindRun      TrajectoryKind = "run"
	TrajectoryKindEndpoint TrajectoryKind = "endpoint"
	TrajectoryKindModel    TrajectoryKind = "model"
	TrajectoryKindItem     TrajectoryKind = "item"
)

// TrajectoryEntry is one timeline record projecting exactly one durable fact.
type TrajectoryEntry struct {
	OccurredAt time.Time
	Source     TrajectorySource
}

// TrajectorySource is closed to the four timeline facts below.
type TrajectorySource interface {
	trajectoryKey() (TrajectoryKind, string)
}

type TrajectoryRun struct{ Run run.Run }

type TrajectoryModel struct {
	RunID      identity.RunID
	Invocation runs.ModelInvocationCommit
}

type TrajectoryEndpoint struct{ Observation modelref.EndpointObservation }

type TrajectoryItem struct{ Item transcript.Item }

func (s TrajectoryRun) trajectoryKey() (TrajectoryKind, string) {
	return TrajectoryKindRun, s.Run.ID().String()
}

func (s TrajectoryModel) trajectoryKey() (TrajectoryKind, string) {
	return TrajectoryKindModel, s.Invocation.CallID
}

func (s TrajectoryEndpoint) trajectoryKey() (TrajectoryKind, string) {
	return TrajectoryKindEndpoint, s.Observation.Snapshot().Invocation.ID
}

func (s TrajectoryItem) trajectoryKey() (TrajectoryKind, string) {
	return TrajectoryKindItem, s.Item.ID().String()
}

func (e TrajectoryEntry) Position() TrajectoryPosition {
	kind, id := e.Source.trajectoryKey()
	return TrajectoryPosition{OccurredAt: e.OccurredAt.UnixNano(), Kind: kind, ID: id}
}

type TrajectoryPosition struct {
	OccurredAt int64
	Kind       TrajectoryKind
	ID         string
}

func (p TrajectoryPosition) key() []string {
	return []string{strconv.FormatInt(p.OccurredAt, 10), string(p.Kind), p.ID}
}

func (p TrajectoryPosition) validate() error {
	switch p.Kind {
	case TrajectoryKindRun:
		return identity.ValidateRun(p.ID)
	case TrajectoryKindModel, TrajectoryKindEndpoint:
		return runs.ValidateInvocation(p.ID)
	case TrajectoryKindItem:
		return identity.ValidateItem(p.ID)
	default:
		return errors.New("sessions: unknown trajectory kind")
	}
}

// queryTrajectoryReader returns a bounded page hydrated from one database
// snapshot, including the Session existence check and descendant filter.
type queryTrajectoryReader interface {
	PageTrajectory(context.Context, identity.SessionID, bool, *TrajectoryPosition, int) ([]TrajectoryEntry, error)
}

func (c *QueryCoordinator) ListTrajectoryPage(ctx context.Context, sessionID identity.SessionID, includeDescendants bool, cursor string, limit pagination.RequestedLimit) (pagination.Page[TrajectoryEntry], error) {
	const namespace = "session-trajectory"
	if err := sessionID.Validate(); err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	filters := []string{sessionID.String(), strconv.FormatBool(includeDescendants)}
	key, err := pagination.Decode(cursor, namespace, filters)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	anchor, err := trajectoryAnchor(key)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	size, err := limit.Resolve(trajectoryPageLimit)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	rows, err := c.trajectory.PageTrajectory(ctx, sessionID, includeDescendants, anchor, size+1)
	if err != nil {
		return pagination.Page[TrajectoryEntry]{}, err
	}
	return pagination.PageOf(rows, size, namespace, filters, func(entry TrajectoryEntry) []string {
		return entry.Position().key()
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
