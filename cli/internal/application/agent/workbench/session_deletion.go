package workbench

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
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
	Phase     SessionDeletionPhase `json:"phase"`
	CommandID agent.CommandID      `json:"commandId,omitempty"`
	SessionID string               `json:"sessionId"`
	Replay    commandreplay.Guard  `json:"replay"`
}

func (p PendingSessionDeletion) validate() error {
	if err := runtimeprotocol.ValidateSessionID(p.SessionID); err != nil {
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
func (p PendingSessionDeletion) Request() agent.DeleteSession {
	return agent.DeleteSession{CommandID: p.CommandID, SessionID: p.SessionID}
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
func (s *Store) StageSessionDeletion(request agent.DeleteSession, replay commandreplay.Guard) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CommandID == "" {
		return errors.New("stage session deletion: command id is empty")
	}
	if err := replay.Validate(); err != nil {
		return err
	}
	pending := PendingSessionDeletion{
		Phase: SessionDeletionPrepared, CommandID: request.CommandID, SessionID: request.SessionID,
		Replay: replay,
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
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return err
	}
	s.sessionDeletions = next
	return nil
}

// RejectSessionDeletion retires a prepared journal after the runtime
// definitively reports that the session still exists and was not deleted.
func (s *Store) RejectSessionDeletion(sessionID string, commandID agent.CommandID) error {
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
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
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return err
	}
	s.sessionDeletions = next
	return nil
}

// ConfirmSessionDeletion converts the exact prepared command into a durable
// local tombstone, then retires all CLI-owned state. Once the tombstone is
// durable, physical cleanup is best-effort: an undeletable old aggregate can
// no longer be observed and will be retried on the next Open.
func (s *Store) ConfirmSessionDeletion(sessionID string, commandID agent.CommandID) error {
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.sessionDeletions[sessionID]
	if !exists || pending.Phase != SessionDeletionPrepared || pending.CommandID != commandID {
		return errors.New("session deletion journal does not match the confirmed command")
	}
	return s.retireSessionStateLocked(sessionID, pending)
}

// ActivateSessionState establishes that the runtime once again owns this
// identity before authoring state is loaded for it. A confirmed deletion must
// finish removing its old aggregate and tombstone first, preventing a later
// import with the same session ID from inheriting obsolete local state.
func (s *Store) ActivateSessionState(sessionID string) error {
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
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
	if err := s.remove(s.sessionStateName(sessionID)); err != nil {
		return fmt.Errorf("remove retired session state: %w", err)
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return fmt.Errorf("clear retired session tombstone: %w", err)
	}
	s.sessionDeletions = next
	return nil
}

// RetireSessionState atomically removes every authoring concern owned by one
// session: its draft, pending run commands, pending HITL and steer commands,
// and rollback journal. Session deletion cannot safely compose the narrower
// mutations because each rewrites the same durable aggregate. A failure between
// them would expose a partially retired session after restart.
func (s *Store) RetireSessionState(sessionID string) error {
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
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
		Replay: commandreplay.UnprotectedGuard(),
	}
	if pending.Phase != SessionDeletionConfirmed {
		next := cloneSessionDeletions(s.sessionDeletions)
		next[sessionID] = confirmed
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
			return fmt.Errorf("save retired session tombstone: %w", err)
		}
		s.sessionDeletions = next
	}
	delete(s.drafts, sessionID)
	delete(s.pendingRuns, sessionID)
	delete(s.pendingResumes, sessionID)
	delete(s.pendingRollbacks, sessionID)
	delete(s.pendingSteers, sessionID)
	if err := s.remove(s.sessionStateName(sessionID)); err != nil {
		return nil
	}
	next := cloneSessionDeletions(s.sessionDeletions)
	delete(next, sessionID)
	if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
		return nil
	}
	s.sessionDeletions = next
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

// recoverConfirmedSessionDeletions performs cleanup which is no longer on the
// correctness path. Failures deliberately leave the tombstone in place; the
// obsolete aggregate remains unreachable and a later Open retries it.
func (s *Store) recoverConfirmedSessionDeletions() {
	for _, pending := range sortedSessionDeletions(s.sessionDeletions) {
		if pending.Phase != SessionDeletionConfirmed {
			continue
		}
		if err := s.remove(s.sessionStateName(pending.SessionID)); err != nil {
			continue
		}
		next := cloneSessionDeletions(s.sessionDeletions)
		delete(next, pending.SessionID)
		if err := s.save(sessionDeletionsName, sortedSessionDeletions(next)); err != nil {
			continue
		}
		s.sessionDeletions = next
	}
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
