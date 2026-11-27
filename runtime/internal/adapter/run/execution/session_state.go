package execution

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

type TransientSessionState struct {
	tools  *toolset.Resolver
	shells *exec.Shells
}

func NewTransientSessionState(
	tools *toolset.Resolver,
	shells *exec.Shells,
) (*TransientSessionState, error) {
	if tools == nil {
		return nil, errors.New("execution: tool resolver is required")
	}
	if shells == nil {
		return nil, errors.New("execution: shells are required")
	}
	return &TransientSessionState{tools: tools, shells: shells}, nil
}

func (s *TransientSessionState) QuiesceSession(sessionID identity.SessionID) error {
	return s.shells.StopSession(sessionID.String())
}

func (s *TransientSessionState) QuiesceWorkspace(root string) error {
	return s.shells.StopWorkspace(root)
}

func (s *TransientSessionState) ForgetSessionContext(sessionID identity.SessionID) {
	s.tools.ForgetSessionContext(sessionID)
}

func (s *TransientSessionState) ForgetWorkspace(root string) {
	s.tools.ForgetWorkspace(root)
}
