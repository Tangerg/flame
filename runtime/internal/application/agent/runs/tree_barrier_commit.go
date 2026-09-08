package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// TreeBarrierCommit is the one durable write-set produced when any executor
// interruption stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one waiting Run commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	commitID   runtimeidentity.CommitID
	pending    Pending
	runs       []EventCommit
	checkpoint run.Checkpoint
}

// NewTreeBarrierCommit binds the root-owned waiting hand-off, opaque executor
// checkpoint, and every Run suspension into one immutable transaction.
func NewTreeBarrierCommit(
	commitID runtimeidentity.CommitID,
	pending Pending,
	runs []EventCommit,
	checkpoint run.Checkpoint,
) (TreeBarrierCommit, error) {
	barrier := TreeBarrierCommit{
		commitID: commitID, pending: pending.Clone(),
		runs: slices.Clone(runs), checkpoint: checkpoint,
	}
	if err := barrier.Validate(); err != nil {
		return TreeBarrierCommit{}, err
	}
	return barrier, nil
}

// Validate proves that the barrier is the complete interruption projection for
// the pending continuation tree and that its checkpoint belongs to the same
// run. The Effects port only persists this already-defined write-set.
func (t TreeBarrierCommit) Validate() error {
	if err := t.commitID.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier: %w", err)
	}
	if err := t.pending.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Pending: %w", err)
	}
	rootContinuation, found := t.pending.RootContinuation()
	if !found {
		return errors.New("runs: tree barrier has no root continuation")
	}
	validator := treeBarrierValidator{
		barrier:       t,
		continuations: make(map[string]Continuation, len(t.pending.Continuations)),
		seenRunIDs:    make(map[string]struct{}, len(t.runs)),
	}
	for _, continuation := range t.pending.Continuations {
		validator.continuations[continuation.RunID] = continuation
	}
	if err := validator.validateCheckpoint(rootContinuation); err != nil {
		return err
	}
	return validator.validateRuns()
}

// CommitID returns the stable tree-barrier transaction identity.
func (t TreeBarrierCommit) CommitID() runtimeidentity.CommitID { return t.commitID }

// Pending returns an isolated snapshot of the complete waiting hand-off.
func (t TreeBarrierCommit) Pending() Pending { return t.pending.Clone() }

// Runs returns isolated nested Run commits in canonical tree postorder.
func (t TreeBarrierCommit) Runs() []EventCommit { return slices.Clone(t.runs) }

// Checkpoint returns an isolated copy of the opaque executor continuation.
func (t TreeBarrierCommit) Checkpoint() run.Checkpoint { return t.checkpoint }

type treeBarrierValidator struct {
	barrier       TreeBarrierCommit
	continuations map[string]Continuation
	seenRunIDs    map[string]struct{}
}

func (t treeBarrierValidator) validateCheckpoint(rootContinuation Continuation) error {
	checkpoint := t.barrier.checkpoint
	pending := t.barrier.pending
	if err := checkpoint.ValidateOwnership(rootContinuation.MemberID, pending.SessionID); err != nil {
		return fmt.Errorf("runs: tree barrier checkpoint ownership: %w", err)
	}
	if checkpoint.Scope().GoalIncarnationID != pending.GoalIncarnationID {
		return fmt.Errorf(
			"runs: tree barrier checkpoint goal incarnation %q does not match Pending %q: %w",
			checkpoint.Scope().GoalIncarnationID,
			pending.GoalIncarnationID,
			run.ErrInvalidCheckpoint,
		)
	}
	if !checkpoint.ModelSelection().Equal(rootContinuation.ModelSelection) {
		return fmt.Errorf("runs: tree barrier checkpoint model differs from root continuation: %w", run.ErrInvalidCheckpoint)
	}
	if checkpoint.Limits() != rootContinuation.Limits {
		return fmt.Errorf("runs: tree barrier checkpoint limits differ from root continuation: %w", run.ErrInvalidCheckpoint)
	}
	return nil
}

func (t treeBarrierValidator) validateRuns() error {
	if len(t.barrier.runs) != len(t.barrier.pending.Continuations) {
		return fmt.Errorf(
			"runs: tree barrier has %d Run commits for %d continuations",
			len(t.barrier.runs),
			len(t.barrier.pending.Continuations),
		)
	}
	for index, runCommit := range t.barrier.runs {
		if err := t.validateRun(index, runCommit); err != nil {
			return err
		}
	}
	return nil
}

func (t treeBarrierValidator) validateRun(index int, runCommit EventCommit) error {
	if !runCommit.CommitID().IsZero() {
		return fmt.Errorf("runs: tree barrier Run[%d] carries a top-level event commit identity", index)
	}
	if runCommit.IsZero() {
		return fmt.Errorf("runs: tree barrier Run[%d] is required", index)
	}
	if !runCommit.Suspends() {
		return fmt.Errorf("runs: tree barrier Run[%d] is not a waiting Run projection", index)
	}
	record := runCommit.Run()
	pending := t.barrier.pending
	if runCommit.SessionID() != pending.SessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
	}
	continuation, exists := t.continuations[runCommit.RunID()]
	if !exists {
		return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
	}
	if record.Lineage() != continuation.Lineage ||
		!record.ModelSelection().Equal(continuation.ModelSelection) ||
		!record.CreatedAt().Equal(continuation.RunCreatedAt) ||
		!record.Metrics().Equal(continuation.Metrics) ||
		record.Limits() != continuation.Limits {
		return fmt.Errorf("runs: tree barrier Run[%d] differs from its continuation", index)
	}
	if !record.Capabilities().Equal(pending.Capabilities) {
		return fmt.Errorf("runs: tree barrier Run[%d] capabilities differ from Pending", index)
	}
	if runCommit.RunID() == pending.RootRunID {
		if record.GoalIncarnationID() != pending.GoalIncarnationID {
			return errors.New("runs: tree barrier root Run goal incarnation differs from Pending")
		}
	} else if record.GoalIncarnationID() != "" {
		return fmt.Errorf("runs: tree barrier child Run[%d] carries a root Goal incarnation", index)
	}
	if _, duplicate := t.seenRunIDs[runCommit.RunID()]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID())
	}
	t.seenRunIDs[runCommit.RunID()] = struct{}{}
	return nil
}
