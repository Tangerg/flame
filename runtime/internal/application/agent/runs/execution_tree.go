package runs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
)

var (
	ErrExecutionTreeCommitConflict = errors.New("runs: execution tree commit conflict")
	ErrExecutionTreeWriterConflict = errors.New("runs: execution tree writer conflict")
)

// ExecutionTreeHead is the executor's opaque recovery cut. The expected writer
// and digest fence replacements; sequence and commit identity distinguish
// repeated content from a retry of the current acknowledged transition.
type ExecutionTreeHead struct {
	SessionID    string
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
	if u.Head.CommitID == "" || u.Head.CommitDigest == "" || u.Head.SessionID == "" || u.Head.RootID == "" || u.Head.Writer == "" || u.Head.Digest == "" || len(u.Head.Payload) == 0 || (u.PreviousWriter == "") != (u.PreviousDigest == "") {
		return errors.New("runs: invalid execution tree update")
	}
	if u.Head.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(u.Head.Payload)) {
		return errors.New("runs: execution tree content differs from its digest")
	}
	return nil
}

type ExecutionTreeStore interface {
	ExecutionResultCommitted(context.Context, string, ResultPublication) (bool, error)
	LoadExecutionTree(context.Context, string, string) (ExecutionTreeHead, bool, error)
	SaveExecutionTree(context.Context, ExecutionTreeUpdate) error
}

// ExecutionTreeSettled commits the recovery head and its newly known product
// facts together. A receipt authorizes Scope to advance; a projection event does not.
type ExecutionTreeSettled struct {
	executionFactBase
	Update ExecutionTreeUpdate
	Facts  []ExecutorEvent
}

func (e ExecutionTreeSettled) clone() ExecutionTreeSettled {
	e.Update.Head.Payload = slices.Clone(e.Update.Head.Payload)
	e.Facts = slices.Clone(e.Facts)
	for index := range e.Facts {
		e.Facts[index].Payload, _ = cloneExecutionFact(e.Facts[index].Payload.(ExecutionFact))
	}
	return e
}

func (h ExecutionTreeHead) SameCommit(other ExecutionTreeHead) bool {
	return h.SessionID == other.SessionID && h.RootID == other.RootID && h.Writer == other.Writer && h.Sequence == other.Sequence && h.CommitID == other.CommitID && h.CommitDigest == other.CommitDigest && h.Digest == other.Digest && bytes.Equal(h.Payload, other.Payload)
}
