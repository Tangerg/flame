package runs

import (
	"bytes"
	"context"
	"errors"
	"slices"

	scoperun "github.com/Tangerg/scope/agent/run"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var (
	ErrExecutionTreeCommitConflict = errors.New("runs: execution tree commit conflict")
	ErrExecutionTreeWriterConflict = errors.New("runs: execution tree writer conflict")
)

type ExecutionTreeHead struct {
	SessionID    identity.SessionID
	RootID       string
	Writer       string
	Digest       string
	Payload      []byte
	Sequence     uint64
	CommitID     string
	CommitDigest string
}

type ExecutionTreeUpdate struct {
	PreviousWriter string
	PreviousDigest string
	Head           ExecutionTreeHead
}

func (u ExecutionTreeUpdate) Validate() error {
	if u.Head.CommitID == "" || u.Head.CommitDigest == "" || u.Head.SessionID.IsZero() || u.Head.RootID == "" || u.Head.Writer == "" || u.Head.Digest == "" || len(u.Head.Payload) == 0 || (u.PreviousWriter == "") != (u.PreviousDigest == "") {
		return errors.New("runs: invalid execution tree update")
	}
	if u.Head.Digest != scoperun.ComputeDigest(u.Head.Payload).String() {
		return errors.New("runs: execution tree content differs from its digest")
	}
	return nil
}

// Stores evaluate Admit inside the write that replaces the head, so two writers
// cannot both advance from the same predecessor. A commit already recorded
// under the update's id is accepted only as an identical replay.
// ExecutionTreeObservation is what storage holds for an update's root. The zero
// value is an absent tree with no recorded commit.
type ExecutionTreeObservation struct {
	current        ExecutionTreeHead
	found          bool
	recordedDigest string
	recorded       bool
}

func ObservedExecutionTree(current ExecutionTreeHead) ExecutionTreeObservation {
	return ExecutionTreeObservation{current: current, found: true}
}

func (o ExecutionTreeObservation) WithRecordedCommit(digest string) ExecutionTreeObservation {
	o.recordedDigest, o.recorded = digest, true
	return o
}

func (u ExecutionTreeUpdate) Admit(observed ExecutionTreeObservation) (replayed bool, err error) {
	next := u.Head
	current, found, recordedDigest, recorded := observed.current, observed.found, observed.recordedDigest, observed.recorded
	if found && u.PreviousWriter == next.Writer && current.Writer != next.Writer {
		return false, ErrExecutionTreeWriterConflict
	}
	if recorded {
		if found && recordedDigest == next.CommitDigest && current.SameCommit(next) {
			return true, nil
		}
		return false, ErrExecutionTreeCommitConflict
	}
	if current.Writer != u.PreviousWriter {
		return false, ErrExecutionTreeWriterConflict
	}
	if current.Digest != u.PreviousDigest || next.Sequence == 0 || found && next.Sequence <= current.Sequence {
		return false, ErrExecutionTreeCommitConflict
	}
	return false, nil
}

type ExecutionTreeStore interface {
	ExecutionResultCommitted(context.Context, identity.SessionID, ResultPublication) (bool, error)
	LoadExecutionTree(context.Context, identity.SessionID, string) (ExecutionTreeHead, bool, error)
	SaveExecutionTree(context.Context, ExecutionTreeUpdate) error
}

type ExecutionTreeSettled struct {
	Context          *ExecutorCheckpoint
	InputAcceptances []CommandAcceptance
	executionFactBase
	Update   ExecutionTreeUpdate
	Children []ChildRunPublication
	Facts    []ExecutorEvent
}

func (e ExecutionTreeSettled) AdvancesHeadOnly() bool {
	return len(e.Facts) == 0 && len(e.Children) == 0 && e.Context == nil && len(e.InputAcceptances) == 0
}

type ChildRunPublication struct {
	Binding ChildRunBinding
	Outcome ChildRunStartOutcome
}

type ExecutionTreeCommit struct {
	Context          *ExecutorCheckpoint
	InputAcceptances []CommandAcceptance
	Update           ExecutionTreeUpdate
	StartedChildren  []StartedChildRun
	AbortedChildren  []ChildRunStartReservation
	Events           []EventCommit
}

func (u ExecutionTreeUpdate) validateSteerAcceptance(acceptance CommandAcceptance) error {
	if acceptance.Operation != CommandSteerInput || acceptance.SessionID != u.Head.SessionID {
		return errors.New("runs: foreign steer acceptance")
	}
	return acceptance.Validate()
}

func (t ExecutionTreeCommit) Validate() error {
	if err := t.Update.Validate(); err != nil {
		return err
	}
	if t.Context != nil {
		if err := t.Context.ValidateOwnership(t.Update.Head.RootID, t.Update.Head.SessionID); err != nil {
			return err
		}
	}
	for _, acceptance := range t.InputAcceptances {
		if err := t.Update.validateSteerAcceptance(acceptance); err != nil {
			return err
		}
	}
	return t.validatePublications()
}

func (t ExecutionTreeCommit) validatePublications() error {
	for _, started := range t.StartedChildren {
		if started.Reservation().SessionID != t.Update.Head.SessionID {
			return errors.New("runs: foreign child Run publication")
		}
	}
	for _, aborted := range t.AbortedChildren {
		if aborted.SessionID != t.Update.Head.SessionID {
			return errors.New("runs: foreign child Run withdrawal")
		}
	}
	for index, commit := range t.Events {
		if commit.SessionID != t.Update.Head.SessionID || commit.CommitID.IsZero() || commit.State == StateSuspend {
			return errors.New("runs: invalid execution tree product commit")
		}
		if err := commit.Validate(); err != nil {
			return err
		}
		if commit.State == StateTerminalize && commit.Run.Lineage().IsRoot() &&
			(commit.ObsoleteCheckpointRootID != t.Update.Head.RootID || index != len(t.Events)-1) {
			return errors.New("runs: execution tree requires its root terminal after child facts")
		}
	}
	return nil
}

func (e ExecutionTreeSettled) clone() (ExecutionTreeSettled, bool) {
	e.InputAcceptances = slices.Clone(e.InputAcceptances)
	for index := range e.InputAcceptances {
		e.InputAcceptances[index] = e.InputAcceptances[index].Clone()
	}
	if e.Context != nil {
		e.Context = new(e.Context.Clone())
	}
	e.Update.Head.Payload = slices.Clone(e.Update.Head.Payload)
	e.Children = slices.Clone(e.Children)
	e.Facts = slices.Clone(e.Facts)
	for index := range e.Facts {
		var fact ExecutionFact
		switch value := e.Facts[index].Payload.(type) {
		case ToolResultsCommitted:
			fact = value
		case SegmentEnded:
			fact = value
		default:
			return ExecutionTreeSettled{}, false
		}
		owned, supported := cloneExecutionFact(fact)
		if !supported {
			return ExecutionTreeSettled{}, false
		}
		e.Facts[index].Payload = owned
	}
	return e, true
}

func (h ExecutionTreeHead) SameCommit(other ExecutionTreeHead) bool {
	return h.SessionID == other.SessionID && h.RootID == other.RootID && h.Writer == other.Writer && h.Sequence == other.Sequence && h.CommitID == other.CommitID && h.CommitDigest == other.CommitDigest && h.Digest == other.Digest && bytes.Equal(h.Payload, other.Payload)
}
