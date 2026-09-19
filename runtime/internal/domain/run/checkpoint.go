package run

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ErrCheckpointNotFound reports that no durable executor checkpoint
// exists for the requested root member identity.
var ErrCheckpointNotFound = errors.New("executor checkpoint not found")

// ErrInvalidCheckpoint reports malformed host-owned metadata around
// an executor's opaque continuation state.
var ErrInvalidCheckpoint = errors.New("invalid executor checkpoint")

// ExecutionScope is the immutable host context shared by a root execution and
// every delegated child. Sessions, workspace isolation, and autonomous-goal
// leases are host facts, not planner state.
type ExecutionScope struct {
	SessionID         string
	CWD               string
	WorkspaceCWD      string
	Isolated          bool
	GoalIncarnationID string
}

// Validate rejects ambiguous host identities before they cross a durable
// continuation boundary.
func (e ExecutionScope) Validate() error {
	if _, err := resourceid.ParseSession(e.SessionID); err != nil {
		return fmt.Errorf("execution: scope: %w", err)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "working dir", value: e.CWD},
		{name: "workspace dir", value: e.WorkspaceCWD},
	} {
		if field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("execution: scope %s has surrounding whitespace", field.name)
		}
	}
	if _, _, err := goalref.ParseOptionalIncarnation(e.GoalIncarnationID); err != nil {
		return fmt.Errorf("execution: scope: %w", err)
	}
	return nil
}

// Checkpoint is one root-owned durable continuation aggregate. Payload
// contains the complete executor tree and is opaque outside its executor
// implementation; the host owns only the aggregate identity and metadata needed
// to decide whether and how the continuation may be restored.
type Checkpoint struct{ state CheckpointState }

// CheckpointState is the construction and persistence boundary of a continuation.
type CheckpointState struct {
	RootMemberID   string
	Payload        []byte
	BuildID        string
	Scope          ExecutionScope
	ModelSelection modelref.Selection
	Limits         Limits
	Capabilities   Capabilities
	Usage          accounting.Snapshot
}

// CheckpointExpectation is the durable identity and host context a
// durable continuation must still belong to before it may be retained or
// restored. It contains no executor topology: every field is independently
// known by the owning Run and Session.
type CheckpointExpectation struct {
	RootMemberID      string
	SessionID         string
	CWD               string
	WorkspaceCWD      string
	Isolated          bool
	GoalIncarnationID string
	ModelSelection    modelref.Selection
	Limits            Limits
	Capabilities      Capabilities
}

// NewCheckpoint validates and owns the complete continuation boundary.
func NewCheckpoint(state CheckpointState) (Checkpoint, error) {
	e := Checkpoint{state: state}
	if err := e.validateInitialState(); err != nil {
		return Checkpoint{}, err
	}
	state.Payload = append([]byte(nil), state.Payload...)
	state.Capabilities = state.Capabilities.Clone()
	state.Usage.Models = append([]accounting.ModelUsage(nil), state.Usage.Models...)
	e.state = state
	return e, nil
}

func (e Checkpoint) IsZero() bool                       { return e.state.RootMemberID == "" }
func (e Checkpoint) RootMemberID() string               { return e.state.RootMemberID }
func (e Checkpoint) BuildID() string                    { return e.state.BuildID }
func (e Checkpoint) Scope() ExecutionScope              { return e.state.Scope }
func (e Checkpoint) ModelSelection() modelref.Selection { return e.state.ModelSelection }
func (e Checkpoint) Limits() Limits                     { return e.state.Limits }
func (e Checkpoint) Capabilities() Capabilities         { return e.state.Capabilities.Clone() }
func (e Checkpoint) Payload() []byte                    { return append([]byte(nil), e.state.Payload...) }
func (e Checkpoint) Usage() accounting.Snapshot {
	usage := e.state.Usage
	usage.Models = append([]accounting.ModelUsage(nil), usage.Models...)
	return usage
}

// State returns owned data for persistence and reconstruction.
func (e Checkpoint) State() CheckpointState {
	if e.IsZero() {
		return CheckpointState{}
	}
	state := e.state
	state.Payload, state.Capabilities, state.Usage = e.Payload(), e.Capabilities(), e.Usage()
	return state
}

// ValidateSuccessor protects the identity, frozen policy, and cumulative accounting
// of one execution when a later durable barrier replaces its checkpoint.
func (e Checkpoint) ValidateSuccessor(next Checkpoint) error {
	if e.IsZero() || next.IsZero() {
		return fmt.Errorf("%w: checkpoint is required", ErrInvalidCheckpoint)
	}
	left, right := e.state, next.state
	if left.RootMemberID != right.RootMemberID || left.BuildID != right.BuildID || left.Scope != right.Scope ||
		!left.ModelSelection.Equal(right.ModelSelection) || left.Limits != right.Limits || !left.Capabilities.Equal(right.Capabilities) {
		return fmt.Errorf("%w: execution identity and policy are immutable", ErrInvalidCheckpoint)
	}
	if err := right.Usage.ValidateAdvanceFrom(left.Usage); err != nil {
		return fmt.Errorf("%w: cumulative usage: %w", ErrInvalidCheckpoint, err)
	}
	return nil
}

// Equal reports whether two barriers contain the same continuation and host facts.
func (e Checkpoint) Equal(other Checkpoint) bool {
	if e.IsZero() || other.IsZero() {
		return e.IsZero() && other.IsZero()
	}
	left, right := e.state, other.state
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID && left.Scope == right.Scope &&
		left.ModelSelection.Equal(right.ModelSelection) && left.Limits == right.Limits && left.Capabilities.Equal(right.Capabilities) &&
		bytes.Equal(left.Payload, right.Payload) && slices.Equal(left.Usage.Models, right.Usage.Models)
}

func (e Checkpoint) validateInitialState() error {
	if _, err := runtimeidentity.ParseMember(e.state.RootMemberID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCheckpoint, err)
	}
	if len(e.state.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidCheckpoint)
	}
	if _, err := runtimeidentity.ParseBuild(e.state.BuildID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCheckpoint, err)
	}
	if err := e.state.Scope.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCheckpoint, err)
	}
	if err := e.state.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCheckpoint, err)
	}
	if err := e.state.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: limits: %w", ErrInvalidCheckpoint, err)
	}
	if err := e.state.Capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: capabilities: %w", ErrInvalidCheckpoint, err)
	}
	if err := e.state.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: usage: %w", ErrInvalidCheckpoint, err)
	}
	return nil
}

// ValidateOwnership proves that the checkpoint and its owning Run aggregate
// name the same root member and Session. Callers use this at every atomic
// Pending/checkpoint write boundary so two separately valid values cannot be
// committed as one mismatched continuation.
func (e Checkpoint) ValidateOwnership(rootMemberID, sessionID string) error {
	if e.IsZero() {
		return fmt.Errorf("%w: checkpoint is required", ErrInvalidCheckpoint)
	}
	if _, err := runtimeidentity.ParseMember(rootMemberID); err != nil {
		return fmt.Errorf("%w: expected %v", ErrInvalidCheckpoint, err)
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return fmt.Errorf("%w: expected %v", ErrInvalidCheckpoint, err)
	}
	if e.state.RootMemberID != rootMemberID {
		return fmt.Errorf(
			"%w: root member ID %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.RootMemberID,
			rootMemberID,
		)
	}
	if e.state.Scope.SessionID != sessionID {
		return fmt.Errorf(
			"%w: session ID %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.Scope.SessionID,
			sessionID,
		)
	}
	return nil
}

// ValidateFor proves both ownership and every host fact independently known at
// restore time. This prevents one logical execution from running tools in
// the checkpoint workspace while hooks or delegated work use the Session's
// current workspace.
func (e Checkpoint) ValidateFor(expected CheckpointExpectation) error {
	if err := e.ValidateOwnership(expected.RootMemberID, expected.SessionID); err != nil {
		return err
	}
	if expected.CWD != strings.TrimSpace(expected.CWD) {
		return fmt.Errorf("%w: expected working dir has surrounding whitespace", ErrInvalidCheckpoint)
	}
	if expected.WorkspaceCWD != strings.TrimSpace(expected.WorkspaceCWD) {
		return fmt.Errorf("%w: expected workspace dir has surrounding whitespace", ErrInvalidCheckpoint)
	}
	if err := expected.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("%w: expected %w", ErrInvalidCheckpoint, err)
	}
	if err := expected.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: expected limits: %w", ErrInvalidCheckpoint, err)
	}
	if err := expected.Capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: expected capabilities: %w", ErrInvalidCheckpoint, err)
	}
	if _, _, err := goalref.ParseOptionalIncarnation(expected.GoalIncarnationID); err != nil {
		return fmt.Errorf("%w: expected %v", ErrInvalidCheckpoint, err)
	}
	if e.state.Scope.CWD != expected.CWD {
		return fmt.Errorf(
			"%w: working dir %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.Scope.CWD,
			expected.CWD,
		)
	}
	if e.state.Scope.WorkspaceCWD != expected.WorkspaceCWD {
		return fmt.Errorf(
			"%w: workspace dir %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.Scope.WorkspaceCWD,
			expected.WorkspaceCWD,
		)
	}
	if e.state.Scope.Isolated != expected.Isolated {
		return fmt.Errorf(
			"%w: isolation %t does not match owner %t",
			ErrInvalidCheckpoint,
			e.state.Scope.Isolated,
			expected.Isolated,
		)
	}
	if e.state.Scope.GoalIncarnationID != expected.GoalIncarnationID {
		return fmt.Errorf(
			"%w: goal incarnation ID %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.Scope.GoalIncarnationID,
			expected.GoalIncarnationID,
		)
	}
	if !e.state.ModelSelection.Equal(expected.ModelSelection) {
		return fmt.Errorf(
			"%w: model selection %q does not match owner %q",
			ErrInvalidCheckpoint,
			e.state.ModelSelection,
			expected.ModelSelection,
		)
	}
	if e.state.Limits != expected.Limits {
		return fmt.Errorf(
			"%w: limits %+v do not match owner %+v",
			ErrInvalidCheckpoint,
			e.state.Limits,
			expected.Limits,
		)
	}
	if !e.state.Capabilities.Equal(expected.Capabilities) {
		return fmt.Errorf(
			"%w: capabilities %+v do not match owner %+v",
			ErrInvalidCheckpoint,
			e.state.Capabilities,
			expected.Capabilities,
		)
	}
	return nil
}
