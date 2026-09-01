package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// TreeBarrierCommit is the one durable write-set produced when any executor
// interruption stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	CommitID   runtimeidentity.CommitID
	Pending    Pending
	Runs       []EventCommit
	Checkpoint ExecutorCheckpoint
}

// Validate proves that the barrier is the complete interruption projection for
// the pending continuation tree and that its checkpoint belongs to the same
// run. The Effects port only persists this already-defined write-set.
func (t TreeBarrierCommit) Validate() error {
	if err := t.CommitID.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier: %w", err)
	}
	if err := t.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Pending: %w", err)
	}
	rootContinuation, found := t.Pending.RootContinuation()
	if !found {
		return errors.New("runs: tree barrier has no root continuation")
	}
	validator := treeBarrierValidator{
		barrier:       t,
		continuations: make(map[string]Continuation, len(t.Pending.Continuations)),
		seenRunIDs:    make(map[string]struct{}, len(t.Runs)),
	}
	for _, continuation := range t.Pending.Continuations {
		validator.continuations[continuation.RunID] = continuation
	}
	if err := validator.validateCheckpoint(rootContinuation); err != nil {
		return err
	}
	return validator.validateRuns()
}

type treeBarrierValidator struct {
	barrier       TreeBarrierCommit
	continuations map[string]Continuation
	seenRunIDs    map[string]struct{}
}

func (t treeBarrierValidator) validateCheckpoint(rootContinuation Continuation) error {
	checkpoint := t.barrier.Checkpoint
	pending := t.barrier.Pending
	if err := checkpoint.ValidateOwnership(rootContinuation.MemberID, pending.SessionID); err != nil {
		return fmt.Errorf("runs: tree barrier checkpoint ownership: %w", err)
	}
	if checkpoint.Scope.GoalIncarnationID != pending.GoalIncarnationID {
		return fmt.Errorf(
			"runs: tree barrier checkpoint goal incarnation %q does not match Pending %q: %w",
			checkpoint.Scope.GoalIncarnationID,
			pending.GoalIncarnationID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return fmt.Errorf("runs: tree barrier checkpoint model differs from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	if checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf("runs: tree barrier checkpoint limits differ from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	return nil
}

func (t treeBarrierValidator) validateRuns() error {
	if len(t.barrier.Runs) != len(t.barrier.Pending.Continuations) {
		return fmt.Errorf(
			"runs: tree barrier has %d Run commits for %d continuations",
			len(t.barrier.Runs),
			len(t.barrier.Pending.Continuations),
		)
	}
	for index, runCommit := range t.barrier.Runs {
		if err := t.validateRun(index, runCommit); err != nil {
			return err
		}
	}
	return nil
}

func (t treeBarrierValidator) validateRun(index int, runCommit EventCommit) error {
	if !runCommit.CommitID.IsZero() {
		return fmt.Errorf("runs: tree barrier Run[%d] carries a top-level event commit identity", index)
	}
	if err := runCommit.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Run[%d]: %w", index, err)
	}
	if runCommit.State != StateSuspend || runCommit.Run == nil || runCommit.Run.State() != run.Waiting {
		return fmt.Errorf("runs: tree barrier Run[%d] is not a waiting Run projection", index)
	}
	pending := t.barrier.Pending
	if runCommit.SessionID != pending.SessionID || runCommit.Run.SessionID() != pending.SessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
	}
	continuation, exists := t.continuations[runCommit.RunID]
	if !exists {
		return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
	}
	if runCommit.Run.Lineage() != continuation.Lineage ||
		runCommit.Run.ModelSelection() != continuation.ModelSelection ||
		!runCommit.Run.CreatedAt().Equal(continuation.RunCreatedAt) ||
		!runCommit.Run.Metrics().Equal(continuation.Metrics) ||
		runCommit.Run.Limits() != continuation.Limits {
		return fmt.Errorf("runs: tree barrier Run[%d] differs from its continuation", index)
	}
	if !runCommit.Run.Capabilities().Equal(pending.Capabilities) {
		return fmt.Errorf("runs: tree barrier Run[%d] capabilities differ from Pending", index)
	}
	if runCommit.RunID == pending.RootRunID {
		if runCommit.Run.GoalIncarnationID() != pending.GoalIncarnationID {
			return errors.New("runs: tree barrier root Run goal incarnation differs from Pending")
		}
	} else if runCommit.Run.GoalIncarnationID() != "" {
		return fmt.Errorf("runs: tree barrier child Run[%d] carries a root Goal incarnation", index)
	}
	if _, duplicate := t.seenRunIDs[runCommit.RunID]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID)
	}
	t.seenRunIDs[runCommit.RunID] = struct{}{}
	return nil
}
