package agentexec

import (
	"errors"

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
) (*TransientSessionState, error) {
	if workingContexts == nil || tools == nil || shells == nil {
		return nil, errors.New("agentexec: transient Session state requires working contexts, tools, and shells")
	}
	return &TransientSessionState{workingContexts: workingContexts, tools: tools, shells: shells}, nil
}

// QuiesceSession stops every detached process owned by a Session before its
// durable state is replaced or deleted.
func (s *TransientSessionState) QuiesceSession(sessionID string) error {
	return s.shells.StopSession(sessionID)
}

// QuiesceWorkspace stops every detached process below a working tree before a
// destructive file restore begins.
func (s *TransientSessionState) QuiesceWorkspace(root string) error {
	return s.shells.StopWorkspace(root)
}

// ForgetSession releases non-failing process-local markers after a Session has
// been quiesced and durably deleted.
func (s *TransientSessionState) ForgetSession(sessionID string) {
	s.workingContexts.ForgetSession(sessionID)
	s.ForgetSessionContext(sessionID)
}

// ForgetSessionContext releases only facts derived from one Session's model
// context. Lifecycle-hook delivery remains once per Session per Runtime process.
func (s *TransientSessionState) ForgetSessionContext(sessionID string) {
	s.tools.ForgetSessionContext(sessionID)
}

// ForgetWorkspace releases context-derived facts for every Session that has
// observed files below a restored working tree.
func (s *TransientSessionState) ForgetWorkspace(root string) {
	s.tools.ForgetWorkspace(root)
}
