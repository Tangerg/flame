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

// SessionDeletionPhase separates an unacknowledged runtime mutation from a
// confirmed local tombstone. A confirmed record makes an obsolete session
// aggregate unreachable even when its physical file cannot yet be removed.
type SessionDeletionPhase string

const (
	SessionDeletionPrepared  SessionDeletionPhase = "prepared"
	SessionDeletionConfirmed SessionDeletionPhase = "confirmed"
)

// PendingSessionDeletion is the durable journal for one session deletion.
// Prepared records retain the stable runtime command identity; confirmed
// records remain only while obsolete CLI-local state still needs cleanup.
type PendingSessionDeletion struct {
	Phase     SessionDeletionPhase  `json:"phase"`
	CommandID authoring.CommandID   `json:"commandId,omitzero"`
	SessionID string                `json:"sessionId"`
	Replay    authoring.ReplayGuard `json:"replay"`
}

func (p PendingSessionDeletion) validate() error {
	if err := protocol.ValidateSessionID(p.SessionID); err != nil {
		return fmt.Errorf("session deletion: %w", err)
	}
	switch p.Phase {
	case SessionDeletionPrepared:
		if err := p.CommandID.Validate(); err != nil {
			return fmt.Errorf("session deletion command: %w", err)
		}
	case SessionDeletionConfirmed:
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

// Request reconstructs the exact runtime mutation owned by a prepared record.
func (p PendingSessionDeletion) Request() conversation.DeleteSession {
	return conversation.DeleteSession{CommandID: p.CommandID, SessionID: p.SessionID}
}

func (s *Store) publishSessionDeletions(next map[string]PendingSessionDeletion) {
	s.sessionDeletions = next
	for sessionID, pending := range next {
		if pending.Phase == SessionDeletionConfirmed {
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

// PendingSessionDeletions returns deletion journals in stable session order.
func (s *Store) PendingSessionDeletions() []PendingSessionDeletion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedSessionDeletions(s.sessionDeletions)
}

// PendingSessionDeletion returns the journal for one session, when present.
func (s *Store) PendingSessionDeletion(sessionID string) (PendingSessionDeletion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.sessionDeletions[sessionID]
	return pending, ok
}

// StageSessionDeletion durably owns a deletion and the exact runtime command
// that may replay it after process restart. Repeating the exact command is
// idempotent; another identity cannot replace an unsettled outcome.
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
		Phase: SessionDeletionPrepared, CommandID: request.CommandID, SessionID: request.SessionID,
		Replay: replayGuard,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(request.SessionID) {
		return errors.New("session draft transfer requires recovery")
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

// RejectSessionDeletion retires a prepared journal after the runtime
// definitively reports that the session still exists and was not deleted.
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
	if pending.Phase != SessionDeletionPrepared || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the rejected command")
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	return s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) })
}

// ConfirmSessionDeletion converts the exact prepared command into a durable
// local tombstone, then retires all CLI-owned state. A cleanup failure preserves
// the tombstone and its cause; the obsolete aggregate cannot become live again.
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

// ActivateSessionState establishes that the runtime once again owns this
// identity before authoring state is loaded for it. A confirmed deletion must
// finish removing its old aggregate and tombstone first, preventing a later
// import with the same session ID from inheriting obsolete local state.
func (s *Store) ActivateSessionState(sessionID string) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransferBlocks(sessionID) {
		return errors.New("session draft transfer requires recovery")
	}
	pending, exists := s.sessionDeletions[sessionID]
	if !exists {
		return nil
	}
	if pending.Phase == SessionDeletionPrepared {
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

// RetireSessionState atomically removes every authoring concern owned by one
// session: its draft, pending run commands, pending HITL and steer commands,
// and rollback journal. Session deletion cannot safely compose the narrower
// mutations because each rewrites the same durable aggregate. A failure between
// them would expose a partially retired session after restart.
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
		return errors.New("session draft transfer requires recovery")
	}
	confirmed := PendingSessionDeletion{
		Phase: SessionDeletionConfirmed, CommandID: pending.CommandID, SessionID: sessionID,
		Replay: authoring.UnprotectedGuard(),
	}
	if pending.Phase != SessionDeletionConfirmed {
		next := cloneSessionDeletions(s.sessionDeletions)
		next[sessionID] = confirmed
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next), func() { s.publishSessionDeletions(next) }); err != nil {
			return fmt.Errorf("save retired session tombstone: %w", err)
		}
	}

	// Retirement is committed by the confirmed tombstone above. It stays until
	// both cleanup writes succeed, so recovery retries a failure here.
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
		if pending.Phase == SessionDeletionConfirmed && filepath.Base(s.sessionStateName(sessionID)) == name {
			return true
		}
	}
	return false
}

func (s *Store) recoverConfirmedSessionDeletions() error {
	for _, pending := range sortedSessionDeletions(s.sessionDeletions) {
		if pending.Phase != SessionDeletionConfirmed {
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
