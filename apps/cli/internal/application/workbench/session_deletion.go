package workbench

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type PendingSessionDeletion struct {
	CommandID authoring.CommandID   `json:"commandId"`
	SessionID string                `json:"sessionId"`
	Replay    authoring.ReplayGuard `json:"replay"`
}

func (p PendingSessionDeletion) validate() error {
	if err := protocol.ValidateSessionID(p.SessionID); err != nil {
		return fmt.Errorf("session deletion: %w", err)
	}
	if err := p.CommandID.Validate(); err != nil {
		return fmt.Errorf("session deletion command: %w", err)
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	return nil
}

func (p PendingSessionDeletion) Request() conversation.DeleteSession {
	return conversation.DeleteSession{CommandID: p.CommandID, SessionID: p.SessionID}
}

func (s *Store) loadSessionDeletions() error {
	var deletions []PendingSessionDeletion
	if err := s.loadOptional(sessionDeletionsName, &deletions); err != nil {
		return fmt.Errorf("load session deletions: %w", err)
	}
	for index, pending := range deletions {
		if err := pending.validate(); err != nil {
			return fmt.Errorf("load session deletion %d: %w", index+1, err)
		}
		if _, duplicate := s.sessionDeletions[pending.SessionID]; duplicate {
			return fmt.Errorf("load session deletions: session %q appears more than once", pending.SessionID)
		}
		s.sessionDeletions[pending.SessionID] = pending
	}
	return nil
}

func (s *Store) PendingSessionDeletions() []PendingSessionDeletion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedSessionDeletions(s.sessionDeletions)
}

func (s *Store) PendingSessionDeletion(sessionID string) (PendingSessionDeletion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.sessionDeletions[sessionID]
	return pending, ok
}

func (s *Store) StageSessionDeletion(request conversation.DeleteSession, replayGuard authoring.ReplayGuard) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CommandID == "" {
		return errors.New("stage session deletion: command id is empty")
	}
	if err := replayGuard.Validate(); err != nil {
		return err
	}
	pending := PendingSessionDeletion{
		CommandID: request.CommandID, SessionID: request.SessionID,
		Replay: replayGuard,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.sessionDeletions[request.SessionID]; exists {
		if current == pending {
			return nil
		}
		return errors.New("another session deletion is already pending")
	}
	next := maps.Clone(s.sessionDeletions)
	next[request.SessionID] = pending
	return s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.sessionDeletions = next })
}

func (s *Store) RejectSessionDeletion(sessionID string, commandID authoring.CommandID) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.sessionDeletions[sessionID]
	if !exists {
		return nil
	}
	if pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the rejected command")
	}
	next := maps.Clone(s.sessionDeletions)
	delete(next, sessionID)
	return s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.sessionDeletions = next })
}

func (s *Store) ConfirmSessionDeletion(sessionID string, commandID authoring.CommandID) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, exists := s.sessionDeletions[sessionID]; exists && pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the confirmed command")
	}
	return s.retireSessionStateLocked(sessionID)
}

func (s *Store) CheckSessionActivation(sessionID string) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessionDeletions[sessionID]; exists {
		return errors.New("session deletion acknowledgement is still pending")
	}
	return nil
}

func (s *Store) RetireSessionState(sessionID string) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retireSessionStateLocked(sessionID)
}

func (s *Store) retireSessionStateLocked(sessionID string) error {
	next := maps.Clone(s.sessionDeletions)
	delete(next, sessionID)
	record, err := encodeRecord(sessionDeletionsName, sortedSessionDeletions(next))
	if err != nil {
		return err
	}
	if err := s.commitSessions([]sessionUpdate{{sessionID: sessionID}}, record); err != nil {
		return fmt.Errorf("retire session state: %w", err)
	}
	s.sessionDeletions = next
	return nil
}

func sortedSessionDeletions(source map[string]PendingSessionDeletion) []PendingSessionDeletion {
	deletions := make([]PendingSessionDeletion, 0, len(source))
	for _, pending := range source {
		deletions = append(deletions, pending)
	}
	slices.SortFunc(deletions, func(left, right PendingSessionDeletion) int {
		return strings.Compare(left.SessionID, right.SessionID)
	})
	return deletions
}
