package workbench

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type PendingSessionDeletion struct {
	Phase     CommandPhase          `json:"phase"`
	CommandID authoring.CommandID   `json:"commandId,omitzero"`
	SessionID string                `json:"sessionId"`
	Replay    authoring.ReplayGuard `json:"replay"`
}

func (p PendingSessionDeletion) validate() error {
	if err := protocol.ValidateSessionID(p.SessionID); err != nil {
		return fmt.Errorf("session deletion: %w", err)
	}
	switch p.Phase {
	case CommandPrepared:
		if err := p.CommandID.Validate(); err != nil {
			return fmt.Errorf("session deletion command: %w", err)
		}
	case CommandConfirmed:
		if p.CommandID != "" {
			if err := p.CommandID.Validate(); err != nil {
				return fmt.Errorf("session deletion command: %w", err)
			}
		}
	default:
		return fmt.Errorf("session deletion phase %q is invalid", p.Phase)
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	return nil
}

func (p PendingSessionDeletion) Request() conversation.DeleteSession {
	return conversation.DeleteSession{CommandID: p.CommandID, SessionID: p.SessionID}
}

func (s *Store) publishSessionDeletions(next map[string]PendingSessionDeletion) {
	s.sessionDeletions = next
	for sessionID, pending := range next {
		if pending.Phase == CommandConfirmed {
			s.installSessionState(sessionID, authoring.Message{}, nil, nil, nil, nil)
		}
	}
	s.history = s.trimHistory(s.history)
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
		Phase: CommandPrepared, CommandID: request.CommandID, SessionID: request.SessionID,
		Replay: replayGuard,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(request.SessionID) {
		return errDraftTransferRecovery
	}
	if current, exists := s.sessionDeletions[request.SessionID]; exists {
		if current == pending {
			return nil
		}
		return errors.New("another session deletion is already pending")
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	next[request.SessionID] = pending
	return s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) })
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
	if pending.Phase != CommandPrepared || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the rejected command")
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	return s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) })
}

func (s *Store) ConfirmSessionDeletion(sessionID string, commandID authoring.CommandID) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.sessionDeletions[sessionID]
	if !exists || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the confirmed command")
	}
	return s.retireSessionStateLocked(sessionID, pending)
}

func (s *Store) ActivateSessionState(sessionID string) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(sessionID) {
		return errDraftTransferRecovery
	}
	pending, exists := s.sessionDeletions[sessionID]
	if !exists {
		return nil
	}
	if pending.Phase == CommandPrepared {
		return errors.New("session deletion acknowledgement is still pending")
	}
	if err := s.remove(s.sessionStateName(sessionID), func() {}); err != nil {
		return fmt.Errorf("remove retired session state: %w", err)
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) }); err != nil {
		return fmt.Errorf("clear retired session tombstone: %w", err)
	}

	return nil
}

// Session deletion cannot compose the narrower mutations: each rewrites the same
// durable aggregate, and a failure between them would expose a partially retired
// session after restart.
func (s *Store) RetireSessionState(sessionID string) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.sessionDeletions[sessionID]
	return s.retireSessionStateLocked(sessionID, pending)
}

func (s *Store) retireSessionStateLocked(sessionID string, pending PendingSessionDeletion) error {
	if s.draftTransferBlocks(sessionID) {
		return errDraftTransferRecovery
	}
	confirmed := PendingSessionDeletion{
		Phase: CommandConfirmed, CommandID: pending.CommandID, SessionID: sessionID,
		Replay: authoring.UnprotectedGuard(),
	}
	if pending.Phase != CommandConfirmed {
		next := cloneSessionDeletions(s.sessionDeletions)
		next[sessionID] = confirmed
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) }); err != nil {
			return fmt.Errorf("save retired session tombstone: %w", err)
		}
	}

	if err := s.remove(s.sessionStateName(sessionID), func() {}); err != nil {
		return fmt.Errorf("remove retired session state: %w", err)
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) }); err != nil {
		return fmt.Errorf("clear retired session tombstone: %w", err)
	}

	return nil
}

func (s *Store) confirmedSessionStateFile(name string) bool {
	for sessionID, pending := range s.sessionDeletions {
		if pending.Phase == CommandConfirmed && filepath.Base(s.sessionStateName(sessionID)) == name {
			return true
		}
	}
	return false
}

func (s *Store) recoverConfirmedSessionDeletions() error {
	for _, pending := range sortedSessionDeletions(s.sessionDeletions) {
		if pending.Phase != CommandConfirmed {
			continue
		}
		if err := s.retireSessionStateLocked(pending.SessionID, pending); err != nil {
			return err
		}
	}
	return nil
}

func cloneSessionDeletions(source map[string]PendingSessionDeletion) map[string]PendingSessionDeletion {
	cloned := make(map[string]PendingSessionDeletion, len(source))
	for sessionID, pending := range source {
		cloned[sessionID] = pending
	}
	return cloned
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
