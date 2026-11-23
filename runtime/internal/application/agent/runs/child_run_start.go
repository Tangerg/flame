package runs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type ChildRunStartReservation struct {
	SessionID       identity.SessionID
	ExecutorID      string
	Member          ExecutorMember
	Binding         ChildRunBinding
	SegmentID       identity.SegmentID
	SpawnedByItemID identity.ItemID
	RootRunID       identity.RunID
	ReservedAt      time.Time
}

func (c ChildRunStartReservation) Validate() error {
	if err := c.validateIdentity(); err != nil {
		return err
	}
	if err := c.Member.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation member: %w", err)
	}
	if !c.Member.Child() || c.Member.SpawnCallID == "" {
		return errors.New("runs: child Run start reservation requires a causal child member")
	}
	if err := c.Binding.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation binding: %w", err)
	}
	if c.Binding.MemberID != c.Member.MemberID {
		return errors.New("runs: child Run start reservation member differs from its binding")
	}
	if c.Binding.ParentRunID == c.Binding.RunID ||
		c.Binding.RunID == c.RootRunID {
		return errors.New("runs: child Run start reservation has contradictory Run identity")
	}
	if c.ReservedAt.IsZero() {
		return errors.New("runs: child Run start reservation has no reservation time")
	}
	return instant.Validate(c.ReservedAt)
}

func (c ChildRunStartReservation) validateIdentity() error {
	if err := c.SessionID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := ValidateExecutor(c.ExecutorID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := c.SegmentID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := c.SpawnedByItemID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := c.RootRunID.Validate(); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	return nil
}

func (c ChildRunStartReservation) draft(model flamemodel.Selection, startedAt time.Time) rundomain.Draft {
	return rundomain.Draft{
		RunID:           c.Binding.RunID,
		SessionID:       c.SessionID,
		SpawnedByItemID: c.SpawnedByItemID,
		ParentRunID:     c.Binding.ParentRunID,
		RootRunID:       c.RootRunID,
		SegmentID:       c.SegmentID,
		ModelSelection:  model,
		CreatedAt:       startedAt,
	}
}

type StartedChildRun struct {
	reservation ChildRunStartReservation
	opening     OpeningCommit
}

func NewStartedChildRun(
	reservation ChildRunStartReservation,
	model flamemodel.Selection,
	startedAt time.Time,
	events []EventCommit,
) (StartedChildRun, error) {
	if err := reservation.Validate(); err != nil {
		return StartedChildRun{}, err
	}
	opening, err := NewAdmissionOpeningCommit(
		newRunCommitID(), reservation.draft(model, startedAt), nil, nil, events,
	)
	if err != nil {
		return StartedChildRun{}, err
	}
	return StartedChildRun{reservation: reservation, opening: opening}, nil
}

func (s StartedChildRun) Reservation() ChildRunStartReservation { return s.reservation }

func (s StartedChildRun) Opening() OpeningCommit { return s.opening }

type ChildRunStartCommitter interface {
	ReserveChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
	AbortChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
}

type ChildRunReservationRequest struct {
	executorPayloadBase
	exchange *executorRequest[ChildRunBinding]
}

type ChildRunReservationReceipt struct {
	exchange *executorRequest[ChildRunBinding]
}

func NewChildRunReservationRequest() (ChildRunReservationRequest, ChildRunReservationReceipt) {
	exchange := newExecutorRequest[ChildRunBinding]()
	return ChildRunReservationRequest{exchange: exchange},
		ChildRunReservationReceipt{exchange: exchange}
}

func (c ChildRunReservationRequest) validate() error {
	if c.exchange == nil {
		return errors.New("runs: child Run reservation request has no receipt")
	}
	return nil
}

func (c ChildRunReservationRequest) claim() (claimedReservation, bool) {
	claimed, ok := c.exchange.claim()
	return claimedReservation{request: claimed}, ok
}

type claimedReservation struct {
	request claimedRequest[ChildRunBinding]
}

func (c claimedReservation) grant(binding ChildRunBinding) { c.request.settle(binding, nil) }

func (c claimedReservation) refuse(err error) { c.request.settle(ChildRunBinding{}, err) }

func (c ChildRunReservationReceipt) Await(
	ctx context.Context,
) (ChildRunBinding, error) {
	return c.exchange.await(ctx)
}

type ChildRunStartOutcome string

const (
	ChildRunStarted      ChildRunStartOutcome = "started"
	ChildRunStartAborted ChildRunStartOutcome = "aborted"
)

func (c ChildRunStartOutcome) Valid() bool {
	return c == ChildRunStarted || c == ChildRunStartAborted
}

func (c ChildRunStartOutcome) String() string {
	if !c.Valid() {
		return "invalid"
	}
	return string(c)
}

const MaxActiveChildRuns = 4

type preparedChildStart struct {
	member       ExecutorMember
	parent       *executorRoute
	spawningItem transcript.Item
	route        *executorRoute
	batch        reductionBatch
	started      StartedChildRun
	reservation  ChildRunStartReservation
}

func (p *preparedChildStart) releaseBinding(owner *runTreeOwner) {
	if p == nil || p.route == nil || owner == nil {
		return
	}
	owner.unbindExecutorMember(p.route.runID, p.member.MemberID)
}

func (c *Segments) prepareChildStart(
	spec segmentSpec,
	owner *runTreeOwner,
	routes *executorRoutes,
	member ExecutorMember,
) (*preparedChildStart, error) {
	parent, err := routes.parent(member)
	if err != nil {
		return nil, err
	}
	spawningItem, err := parent.reducer.spawningItem(member.SpawnCallID)
	if err != nil {
		return nil, fmt.Errorf(
			"runs: open child member %q from parent run %q: %w",
			member.MemberID,
			parent.runID,
			err,
		)
	}
	if spawningItem.SessionID() != spec.SessionID {
		return nil, fmt.Errorf(
			"runs: open child member %q spawning item belongs to session %q, want %q",
			member.MemberID,
			spawningItem.SessionID(),
			spec.SessionID,
		)
	}
	childRunID := c.newRunID()
	childSegmentID := c.newSegmentID()
	lineage, validateErr := rundomain.NewLineage(spawningItem.ID(), parent.runID, parent.rootRunID)
	if validateErr != nil {
		return nil, fmt.Errorf("runs: open child member %q lineage: %w", member.MemberID, validateErr)
	}
	child := &executorRoute{
		runID:     childRunID,
		segmentID: childSegmentID,
		rootRunID: parent.rootRunID,
		lineage:   lineage,
	}
	if bindExecutorMemberErr := owner.bindExecutorMember(child.runID, member.MemberID); bindExecutorMemberErr != nil {
		return nil, bindExecutorMemberErr
	}
	release := true
	defer func() {
		if release {
			owner.unbindExecutorMember(child.runID, member.MemberID)
		}
	}()
	reservation := ChildRunStartReservation{
		SessionID: spec.SessionID, ExecutorID: spec.ExecutorID, Member: member,
		Binding: ChildRunBinding{
			MemberID: member.MemberID, RunID: child.runID, ParentRunID: child.lineage.ParentRunID(),
		},
		SegmentID: child.segmentID, SpawnedByItemID: spawningItem.ID(),
		RootRunID: child.rootRunID, ReservedAt: c.publications.nowUTC().UTC(),
	}
	if err := reservation.Validate(); err != nil {
		return nil, err
	}
	release = false
	return &preparedChildStart{
		member: member, parent: parent, spawningItem: spawningItem,
		route: child, reservation: reservation,
	}, nil
}

func (p *preparedChildStart) openingEvents(sessionID identity.SessionID, projected reductionBatch) ([]EventCommit, error) {
	if len(projected.events) == 0 || projected.parkCommit != nil {
		return nil, fmt.Errorf("runs: child member %q produced an invalid opening batch", p.member.MemberID)
	}
	events := []EventCommit{{
		RunID:     p.parent.runID,
		SessionID: sessionID,
		SegmentID: p.parent.segmentID,
		Items:     NewItemWrites(p.spawningItem),
	}}
	for _, reduced := range projected.events {
		if reduced.Event.Terminal() || reduced.Nudge != nil {
			return nil, fmt.Errorf("runs: child member %q produced an invalid opening event", p.member.MemberID)
		}
		if reduced.Commit != nil {
			events = append(events, *reduced.Commit)
		}
	}
	return events, nil
}

func (c *Segments) finalizeChildOpening(
	spec segmentSpec,
	prepared *preparedChildStart,
	startedAt time.Time,
) error {
	if prepared == nil || prepared.route == nil || prepared.parent == nil {
		return errors.New("runs: finalize an incomplete child start")
	}
	if startedAt.IsZero() || startedAt.Location() != time.UTC {
		return errors.New("runs: child opening requires an authoritative UTC start time")
	}
	child := prepared.route
	if child.reducer != nil {
		return fmt.Errorf("runs: child member %q opening was already finalized", prepared.member.MemberID)
	}
	model := prepared.parent.reducer.cfg.Opened.ModelSelection()
	opened, err := rundomain.Admit(prepared.reservation.draft(model, startedAt))
	if err != nil {
		return fmt.Errorf("runs: admit child member %q: %w", prepared.member.MemberID, err)
	}
	child.reducer = newReducer(reducerConfig{
		Opened:       opened,
		WorkspaceCWD: spec.WorkspaceCWD,
		Isolated:     spec.Isolated,
		Now:          c.publications.nowUTC,
	})
	projected, err := child.reducer.open()
	if err != nil {
		return fmt.Errorf("runs: reduce child member %q opening: %w", prepared.member.MemberID, err)
	}
	events, err := prepared.openingEvents(spec.SessionID, projected)
	if err != nil {
		return err
	}
	if err := validateRouteReductionBatch(child, spec.SessionID, projected); err != nil {
		return err
	}
	started, err := NewStartedChildRun(prepared.reservation, model, startedAt, events)
	if err != nil {
		return fmt.Errorf("runs: child member %q opening: %w", prepared.member.MemberID, err)
	}
	prepared.batch = projected
	prepared.started = started
	return nil
}

func (c *Segments) activatePreparedChild(
	spec segmentSpec,
	routes *executorRoutes,
	prepared *preparedChildStart,
) {
	prepared.route.segmentStartedAt = c.publications.nowUTC()
	routes.installChild(prepared.member, prepared.route)
	c.publications.publishRunMoved(spec.SessionID, prepared.route.runID)
}
