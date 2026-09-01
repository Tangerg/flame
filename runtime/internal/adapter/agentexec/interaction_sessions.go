package agentexec

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

// interactionSessions is the sole owner of live executor membership. Closing
// admission freezes the set because no later registration can succeed; failed
// releases remain in the same set for a later shutdown attempt.
type interactionSessions struct {
	mu     sync.Mutex
	live   map[string]*interactionSession
	closed bool
}

func newInteractionSessions() interactionSessions {
	return interactionSessions{live: make(map[string]*interactionSession)}
}

func (s *interactionSessions) register(session *interactionSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("agentexec: Interaction executor is shutting down")
	}
	if _, duplicate := s.live[session.ref.ExecutorID]; duplicate {
		return errors.New("agentexec: duplicate Interaction executor identity")
	}
	s.live[session.ref.ExecutorID] = session
	return nil
}

func (s *interactionSessions) closeAdmission() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *interactionSessions) snapshot() []*interactionSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	targets := make([]*interactionSession, 0, len(s.live))
	for _, session := range s.live {
		targets = append(targets, session)
	}
	slices.SortFunc(targets, func(left, right *interactionSession) int {
		return strings.Compare(left.ref.ExecutorID, right.ref.ExecutorID)
	})
	return targets
}

func (s *interactionSessions) lookup(ref runs.ExecutorRef) (*interactionSession, error) {
	s.mu.Lock()
	session := s.live[ref.ExecutorID]
	s.mu.Unlock()
	if session != nil && session.ref.SessionID != ref.SessionID {
		return nil, runs.ErrInvalidExecutorRef
	}
	return session, nil
}

func (s *interactionSessions) require(ref runs.ExecutorRef) (*interactionSession, error) {
	if err := ref.ValidateFor(ref.SessionID); err != nil {
		return nil, err
	}
	session, err := s.lookup(ref)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("%w: Interaction execution %q", runs.ErrExecutorNotLive, ref.ExecutorID)
	}
	return session, nil
}

func (s *interactionSessions) remove(session *interactionSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live[session.ref.ExecutorID] == session {
		delete(s.live, session.ref.ExecutorID)
	}
}
