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
	commitID   runtimeidentity.CommitID
	pending    Pending
	runs       []EventCommit
	checkpoint ExecutorCheckpoint
}

// NewTreeBarrierCommit binds the root-owned waiting hand-off, opaque executor
// checkpoint, and every Run suspension into one immutable transaction.
func NewTreeBarrierCommit(
	commitID runtimeidentity.CommitID,
	pending Pending,
	runs []EventCommit,
	checkpoint ExecutorCheckpoint,
) (TreeBarrierCommit, error) {
	barrier := TreeBarrierCommit{
		commitID: commitID, pending: pending.Clone(),
		runs: cloneEventCommits(runs), checkpoint: checkpoint.Clone(),
	}
	if err := barrier.Validate(); err != nil {
		return TreeBarrierCommit{}, err
	}
	return barrier, nil
}

func cloneEventCommits(commits []EventCommit) []EventCommit {
	owned := make([]EventCommit, len(commits))
	for index, commit := range commits {
		owned[index] = commit.clone()
	}
	return owned
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
func (t TreeBarrierCommit) Runs() []EventCommit { return cloneEventCommits(t.runs) }

// Checkpoint returns an isolated copy of the opaque executor continuation.
func (t TreeBarrierCommit) Checkpoint() ExecutorCheckpoint { return t.checkpoint.Clone() }

// SessionID is the Session of the tree's root Run, which owns it.
func (t TreeBarrierCommit) SessionID() string {
	for _, commit := range t.runs {
		if commit.RunID == t.pending.RootRunID && commit.Run != nil {
			return commit.Run.SessionID()
		}
	}
	return ""
}

type treeBarrierValidator struct {
	barrier       TreeBarrierCommit
	continuations map[string]Continuation
	seenRunIDs    map[string]struct{}
}

func (t treeBarrierValidator) validateCheckpoint(rootContinuation Continuation) error {
	checkpoint := t.barrier.checkpoint
	if err := checkpoint.ValidateOwnership(rootContinuation.MemberID, t.barrier.SessionID()); err != nil {
		return fmt.Errorf("runs: tree barrier checkpoint ownership: %w", err)
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
	parked := make([]run.Run, 0, len(t.barrier.runs))
	for index, runCommit := range t.barrier.runs {
		if err := t.validateRun(index, runCommit); err != nil {
			return err
		}
		parked = append(parked, *runCommit.Run)
	}
	return validatePendingRunTree(t.barrier.pending, parked)
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
	sessionID := t.barrier.SessionID()
	if runCommit.SessionID != sessionID || runCommit.Run.SessionID() != sessionID {
		return fmt.Errorf("runs: tree barrier Run[%d] Session differs from its root Run", index)
	}
	if _, exists := t.continuations[runCommit.RunID]; !exists {
		return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
	}
	if _, duplicate := t.seenRunIDs[runCommit.RunID]; duplicate {
		return fmt.Errorf("runs: tree barrier repeats Run %q", runCommit.RunID)
	}
	t.seenRunIDs[runCommit.RunID] = struct{}{}
	return nil
}
