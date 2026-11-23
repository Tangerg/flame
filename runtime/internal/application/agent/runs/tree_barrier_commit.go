package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type TreeBarrierCommit struct {
	commitID   CommitID
	pending    Pending
	runs       []EventCommit
	checkpoint ExecutorCheckpoint
}

func NewTreeBarrierCommit(
	commitID CommitID,
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
		continuations: make(map[identity.RunID]Continuation, len(t.pending.Continuations)),
		seenRunIDs:    make(map[identity.RunID]struct{}, len(t.runs)),
	}
	for _, continuation := range t.pending.Continuations {
		validator.continuations[continuation.RunID] = continuation
	}
	if err := validator.validateCheckpoint(rootContinuation); err != nil {
		return err
	}
	return validator.validateRuns()
}

func (t TreeBarrierCommit) CommitID() CommitID { return t.commitID }

func (t TreeBarrierCommit) Pending() Pending { return t.pending.Clone() }

func (t TreeBarrierCommit) Runs() []EventCommit { return cloneEventCommits(t.runs) }

func (t TreeBarrierCommit) Checkpoint() ExecutorCheckpoint { return t.checkpoint.Clone() }

func (t TreeBarrierCommit) SessionID() identity.SessionID {
	for _, commit := range t.runs {
		if commit.RunID == t.pending.RootRunID && commit.Run != nil {
			return commit.Run.SessionID()
		}
	}
	return identity.SessionID{}
}

type treeBarrierValidator struct {
	barrier       TreeBarrierCommit
	continuations map[identity.RunID]Continuation
	seenRunIDs    map[identity.RunID]struct{}
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
