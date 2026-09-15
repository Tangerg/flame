package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

// interactionSessions owns resources from assembly through successful release.
// live indexes only published executors; probes and failed assemblies remain
// owned without becoming callable. Shutdown joins admitted assembly before
// taking its release snapshot.
type interactionSessions struct {
	mu         sync.Mutex
	live       map[string]*interactionSession
	owned      map[*interactionSession]struct{}
	closed     bool
	assembling int
	assembled  chan struct{}
}

func newInteractionSessions() interactionSessions {
	assembled := make(chan struct{})
	close(assembled)
	return interactionSessions{
		live:      make(map[string]*interactionSession),
		owned:     make(map[*interactionSession]struct{}),
		assembled: assembled,
	}
}

func (s *interactionSessions) beginAssembly() (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("agentexec: Interaction executor is shutting down")
	}
	if s.assembling == 0 {
		s.assembled = make(chan struct{})
	}
	s.assembling++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.assembling--
		if s.assembling == 0 {
			close(s.assembled)
		}
	}, nil
}

func (s *interactionSessions) awaitAssembly(ctx context.Context) error {
	s.mu.Lock()
	done := s.assembled
	s.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *interactionSessions) own(session *interactionSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owned[session] = struct{}{}
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
	targets := make([]*interactionSession, 0, len(s.owned))
	for session := range s.owned {
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
	delete(s.owned, session)
	if s.live[session.ref.ExecutorID] == session {
		delete(s.live, session.ref.ExecutorID)
	}
}
