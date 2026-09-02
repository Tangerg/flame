package agentexec

import "github.com/Tangerg/flame/runtime/internal/adapter/toolset"

// TransientSessionState owns process-local execution facts whose validity is
// bounded by a durable Session, its effective model context, or its shared
// working tree. It composes the concrete adapter owners without moving cleanup
// behavior into bootstrap.
type TransientSessionState struct {
	workingContexts *WorkingContextComposer
	tools           *toolset.Resolver
}

// NewTransientSessionState composes the process-local Session state adapters.
func NewTransientSessionState(
	workingContexts *WorkingContextComposer,
	tools *toolset.Resolver,
) *TransientSessionState {
	return &TransientSessionState{workingContexts: workingContexts, tools: tools}
}

// ForgetSession releases every process-local fact owned by a deleted Session.
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
