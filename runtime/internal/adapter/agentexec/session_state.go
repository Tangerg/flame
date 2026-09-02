package agentexec

import (
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

// TransientSessionState owns process-local execution facts whose validity is
// bounded by a durable Session, its effective model context, or its shared
// working tree. It composes the concrete adapter owners without moving cleanup
// behavior into bootstrap.
type TransientSessionState struct {
	workingContexts *WorkingContextComposer
	tools           *toolset.Resolver
	shells          *exec.Shells
}

// NewTransientSessionState composes the process-local Session state adapters.
func NewTransientSessionState(
	workingContexts *WorkingContextComposer,
	tools *toolset.Resolver,
	shells *exec.Shells,
) *TransientSessionState {
	return &TransientSessionState{workingContexts: workingContexts, tools: tools, shells: shells}
}

// QuiesceSession stops every detached process owned by a Session before its
// durable state is replaced or deleted.
func (s *TransientSessionState) QuiesceSession(sessionID string) error {
	if s == nil || s.shells == nil {
		return nil
	}
	return s.shells.StopSession(sessionID)
}

// QuiesceWorkspace stops every detached process below a working tree before a
// destructive file restore begins.
func (s *TransientSessionState) QuiesceWorkspace(root string) error {
	if s == nil || s.shells == nil {
		return nil
	}
	return s.shells.StopWorkspace(root)
}

// ForgetSession releases non-failing process-local markers after a Session has
// been quiesced and durably deleted.
func (s *TransientSessionState) ForgetSession(sessionID string) {
	if s == nil {
		return
	}
	s.workingContexts.ForgetSession(sessionID)
	s.ForgetSessionContext(sessionID)
}

// ForgetSessionContext releases only facts derived from one Session's model
// context. Lifecycle-hook delivery remains once per Session per Runtime process.
func (s *TransientSessionState) ForgetSessionContext(sessionID string) {
	if s == nil || s.tools == nil {
		return
	}
	s.tools.ForgetSessionContext(sessionID)
}

// ForgetWorkspace releases context-derived facts for every Session that has
// observed files below a restored working tree.
func (s *TransientSessionState) ForgetWorkspace(root string) {
	if s == nil || s.tools == nil {
		return
	}
	s.tools.ForgetWorkspace(root)
}
