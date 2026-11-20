package runs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ChildRunStartReservation is the durable, non-public identity allocated for
// one admitted executor child before its executor child is published in the durable tree.
// It is not a Run and must never be returned by Run projections. The exact
// value is consumed when the child either enters Running or aborts.
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

// Validate proves that the reservation binds one child executor identity to
// one future product child without copying executor topology into the Run.
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

// draft is the child Run this reservation admits once its executor child is
// durably published: every identity comes from the reservation, which owns them.
func (c ChildRunStartReservation) draft(model modelchoice.Selection, startedAt time.Time) rundomain.Draft {
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

// StartedChildRun is the write-set that concludes one reservation with the
// Running child Run it admits. The opening is derived from the reservation, so
// the two cannot name different children.
type StartedChildRun struct {
	reservation ChildRunStartReservation
	opening     OpeningCommit
}

// NewStartedChildRun derives the child Run admission from the reservation it
// concludes.
func NewStartedChildRun(
	reservation ChildRunStartReservation,
	model modelchoice.Selection,
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

// Reservation returns the reservation the start concludes.
func (s StartedChildRun) Reservation() ChildRunStartReservation { return s.reservation }

// Opening returns the child Run's admission write-set.
func (s StartedChildRun) Opening() OpeningCommit { return s.opening }

// ChildRunStartCommitter reserves an invisible child identity and withdraws
// unpublished reservations during cleanup. Publication belongs to the atomic
// ExecutionTreeCommit. Exact repeated requests are idempotent; contradictory
// conclusions fail.
type ChildRunStartCommitter interface {
	ReserveChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
	AbortChildRunStart(ctx context.Context, reservation ChildRunStartReservation) error
}

// ChildRunReservationRequest asks the Run pump to durably reserve one child
// Run identity before the corresponding executor child initializes. The event
// envelope carries the opaque child/parent/call causality. The Application
// assigns the reservation time; an executor start time does not exist yet.
type ChildRunReservationRequest struct {
	executorPayloadBase
	exchange *executorRequest[ChildRunBinding]
}

// ChildRunReservationReceipt is the executor's read-only side of one
// child reservation request.
type ChildRunReservationReceipt struct {
	exchange *executorRequest[ChildRunBinding]
}

// NewChildRunReservationRequest creates one single-use reservation handshake.
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

// claimedReservation is the pump's exclusive right to answer one reservation:
// it grants a reserved binding or refuses, never both.
type claimedReservation struct {
	request claimedRequest[ChildRunBinding]
}

// grant answers with a binding taken from a validated reservation.
func (c claimedReservation) grant(binding ChildRunBinding) { c.request.settle(binding, nil) }

func (c claimedReservation) refuse(err error) { c.request.settle(ChildRunBinding{}, err) }

// Await returns the exact product binding after the Application durably stores
// the invisible reservation.
func (c ChildRunReservationReceipt) Await(
	ctx context.Context,
) (ChildRunBinding, error) {
	return c.exchange.await(ctx)
}

// ChildRunStartOutcome identifies the conclusive executor publication
// result applied to one durable reservation. The zero value is invalid.
type ChildRunStartOutcome string

const (
	ChildRunStarted      ChildRunStartOutcome = "started"
	ChildRunStartAborted ChildRunStartOutcome = "aborted"
)

// Valid reports whether c is one conclusive child publication fact.
func (c ChildRunStartOutcome) Valid() bool {
	return c == ChildRunStarted || c == ChildRunStartAborted
}

// String returns the durable child-start conclusion name.
func (c ChildRunStartOutcome) String() string {
	if !c.Valid() {
		return "invalid"
	}
	return string(c)
}

// MaxActiveChildRuns bounds product delegation independently of ordinary Tool
// processes. Reservations count until publication aborts or the drained
// child's authoritative terminal commits.
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

// prepareChildStart allocates and binds one prospective child identity without
// constructing a Run. The actual lifecycle starts only after Scope reports a
// conclusive started outcome with its authoritative start time.
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

// The Runtime admission clock owns the product Run time. The route remains
// invisible until its opening and the executor tree commit together.
// openingEvents commits the parent's spawning Item with the child's opening
// projections; an opening may not finish, park or nudge anything.
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
