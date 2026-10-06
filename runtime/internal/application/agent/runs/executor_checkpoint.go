package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ErrExecutorCheckpointNotFound reports that no durable executor checkpoint
// exists for the requested root member identity.
var ErrExecutorCheckpointNotFound = errors.New("executor checkpoint not found")

// ErrInvalidExecutorCheckpoint reports malformed host-owned metadata around
// an executor's opaque continuation state.
var ErrInvalidExecutorCheckpoint = errors.New("invalid executor checkpoint")

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

// ExecutorCheckpoint is one root-owned durable continuation aggregate. Payload
// contains the complete executor tree and is opaque outside its executor
// implementation; the host owns only the aggregate identity and the metadata
// no other owner records. Model selection, capabilities, and goal incarnation
// belong to the root Run, and workspace and isolation to the Session; restore
// reads them from those owners. Installations is the executor's canonical
// dependency projection; it is stored beside the payload so installation
// admission never interprets continuation state.
type ExecutorCheckpoint struct {
	ToolResultIDs []toolresult.ID
	Installations []plugin.Dependency
	RootMemberID  string
	SessionID     string
	Payload       []byte
	BuildID       string
}

// Clone returns an ownership-independent checkpoint value.
func (e ExecutorCheckpoint) Clone() ExecutorCheckpoint {
	e.ToolResultIDs = slices.Clone(e.ToolResultIDs)
	e.Installations = slices.Clone(e.Installations)
	e.Payload = append([]byte(nil), e.Payload...)
	return e
}

// Validate verifies the host-owned metadata without interpreting the
// executor payload.
func (e ExecutorCheckpoint) Validate() error {
	if err := toolresult.ValidateReferences(e.ToolResultIDs); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := plugin.ValidateDependencies(e.Installations); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := runtimeidentity.ValidateMember(e.RootMemberID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpoint, err)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidExecutorCheckpoint)
	}
	if _, err := runtimeidentity.ParseBuild(e.BuildID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpoint, err)
	}
	if err := resourceid.ValidateSession(e.SessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpoint, err)
	}
	return nil
}

// ValidateOwnership proves that the checkpoint and its owning Run aggregate
// name the same root member and Session. Callers use this at every atomic
// Pending/checkpoint write boundary so two separately valid values cannot be
// committed as one mismatched continuation.
func (e ExecutorCheckpoint) ValidateOwnership(rootMemberID, sessionID string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateMember(rootMemberID); err != nil {
		return fmt.Errorf("%w: expected %v", ErrInvalidExecutorCheckpoint, err)
	}
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return fmt.Errorf("%w: expected %v", ErrInvalidExecutorCheckpoint, err)
	}
	if e.RootMemberID != rootMemberID {
		return fmt.Errorf(
			"%w: root member ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.RootMemberID,
			rootMemberID,
		)
	}
	if e.SessionID != sessionID {
		return fmt.Errorf(
			"%w: session ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.SessionID,
			sessionID,
		)
	}
	return nil
}
