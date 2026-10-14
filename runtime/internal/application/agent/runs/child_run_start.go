package runs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	modelchoice "github.com/Tangerg/flame/runtime/model"
)

// ChildRunStartReservation is the durable, non-public identity allocated for
// one admitted executor child before its executor child is published in the durable tree.
// It is not a Run and must never be returned by Run projections. The exact
// value is consumed when the child either enters Running or aborts.
type ChildRunStartReservation struct {
	SessionID       string
	ExecutorID      string
	Member          ExecutorMember
	Binding         ChildRunBinding
	SegmentID       string
	SpawnedByItemID string
	RootRunID       string
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
	if err := resourceid.ValidateSession(c.SessionID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := runtimeidentity.ValidateExecutor(c.ExecutorID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := resourceid.ValidateSegment(c.SegmentID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := resourceid.ValidateItem(c.SpawnedByItemID); err != nil {
		return fmt.Errorf("runs: child Run start reservation: %w", err)
	}
	if err := resourceid.ValidateRun(c.RootRunID); err != nil {
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
		newRunCommitID(), reservation.draft(model, startedAt), nil, nil, nil, events,
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

func (c ChildRunReservationRequest) claim() bool { return c.exchange.claim() }

// complete hands the reservation's outcome to the waiting child. The receipt is
// the exchange's own question: claim already refused a request without one, and
// the exchange asks again for a caller that skipped it.
func (c ChildRunReservationRequest) complete(binding ChildRunBinding, err error) error {
	if err == nil {
		if validationErr := binding.Validate(); validationErr != nil {
			err = validationErr
		}
	} else if binding != (ChildRunBinding{}) {
		err = errors.Join(err, errors.New("runs: failed child Run reservation returned a binding"))
		binding = ChildRunBinding{}
	}
	return c.exchange.complete(binding, err)
}

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
