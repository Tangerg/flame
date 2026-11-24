package runs

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

var ErrExecutorCheckpointNotFound = errors.New("executor checkpoint not found")

var ErrInvalidExecutorCheckpoint = errors.New("invalid executor checkpoint")

type ExecutionScope struct {
	SessionID         identity.SessionID
	CWD               string
	WorkspaceCWD      string
	Project           identity.ProjectID
	Isolated          bool
	GoalIncarnationID goalref.IncarnationID
}

type ExecutorCheckpoint struct {
	Bindings     []plugin.Activation
	Resources    []plugin.Dependency
	RootMemberID string
	SessionID    identity.SessionID
	Payload      []byte
	BuildID      string
}

func (e ExecutorCheckpoint) Clone() ExecutorCheckpoint {
	e.Bindings = slices.Clone(e.Bindings)
	for index := range e.Bindings {
		e.Bindings[index] = e.Bindings[index].Clone()
	}
	e.Resources = slices.Clone(e.Resources)
	e.Payload = append([]byte(nil), e.Payload...)
	return e
}

func (e ExecutorCheckpoint) Validate() error {
	if err := plugin.ValidateDependencies(e.Resources); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	for _, binding := range e.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
		}
	}
	if err := ValidateMember(e.RootMemberID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidExecutorCheckpoint)
	}
	if _, err := ParseBuild(e.BuildID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := e.SessionID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	return nil
}

func (e ExecutorCheckpoint) Dependencies() []plugin.Dependency {
	result := slices.Clone(e.Resources)
	for _, binding := range e.Bindings {
		result = append(result, binding.Dependencies...)
	}
	return plugin.CompactDependencies(result)
}

func (e ExecutorCheckpoint) ValidateOwnership(rootMemberID string, sessionID identity.SessionID) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := ValidateMember(rootMemberID); err != nil {
		return fmt.Errorf("%w: expected %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := sessionID.Validate(); err != nil {
		return fmt.Errorf("%w: expected %w", ErrInvalidExecutorCheckpoint, err)
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

func (e ExecutorCheckpoint) Equal(other ExecutorCheckpoint) bool {
	return e.RootMemberID == other.RootMemberID && e.SessionID == other.SessionID &&
		e.BuildID == other.BuildID && bytes.Equal(e.Payload, other.Payload) &&
		slices.Equal(e.Resources, other.Resources) &&
		slices.EqualFunc(e.Bindings, other.Bindings, plugin.Activation.Equal)
}
