package workbench

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
)

// PendingRuns returns unacknowledged run-opening commands in authoring order.
func (s *Store) PendingRuns(sessionID string) []PendingRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePendingRunSlice(s.pendingRuns[sessionID])
}

// PendingResume returns the unacknowledged HITL command for one session.
func (s *Store) PendingResume(sessionID string) (PendingResume, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingResumes[sessionID]
	return clonePendingResume(pending), ok
}

// StagePendingResume transfers a completed interaction review into the durable
// command outbox before delivery starts.
func (s *Store) StagePendingResume(sessionID string, pending PendingResume) error {
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	if err := pending.validate(); err != nil {
		return err
	}
	pending = clonePendingResume(pending)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingResumes[sessionID]; exists {
		if current.Command.CommandID == pending.Command.CommandID {
			if !pendingResumeEqual(current, pending) {
				return errors.New("pending resume command identity already owns another decision")
			}
			return nil
		}
		return errors.New("another resume command is already pending")
	}
	next := clonePendingResumes(s.pendingResumes)
	next[sessionID] = pending
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], &pending); err != nil {
		return err
	}
	s.pendingResumes = next
	return nil
}

// AcknowledgePendingResume retires exactly the command whose runtime response
// was observed. A stale callback cannot delete a newer interaction decision.
func (s *Store) AcknowledgePendingResume(sessionID string, commandID agent.CommandID) error {
	return s.retirePendingResume(sessionID, commandID)
}

// RejectPendingResume releases a command after a definitive runtime refusal so
// its review can be edited and submitted under a fresh identity.
func (s *Store) RejectPendingResume(sessionID string, commandID agent.CommandID) error {
	return s.retirePendingResume(sessionID, commandID)
}

// RequeuePendingResume atomically gives a decision a fresh runtime identity
// after the owning store's authoritative waiting projection proves the old
// command did not commit before its replay guarantee expired.
func (s *Store) RequeuePendingResume(
	sessionID string,
	commandID agent.CommandID,
	replay commandreplay.Guard,
) (PendingResume, error) {
	if err := commandID.Validate(); err != nil {
		return PendingResume{}, err
	}
	if err := replay.Validate(); err != nil {
		return PendingResume{}, err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return PendingResume{}, err
	}
	replacement, err := agent.NewCommandID()
	if err != nil {
		return PendingResume{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingResumes[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return PendingResume{}, errors.New("pending resume command identity changed")
	}
	pending = clonePendingResume(pending)
	pending.Command.CommandID = replacement
	pending.Replay = replay
	if err := pending.validate(); err != nil {
		return PendingResume{}, err
	}
	if err := s.saveSessionStateWithResume(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], &pending,
	); err != nil {
		return PendingResume{}, err
	}
	s.pendingResumes[sessionID] = pending
	return clonePendingResume(pending), nil
}

// DiscardPendingResume retires terminal authoring state for a session that the
// runtime has deleted or replaced. It never runs as part of ordinary session
// navigation, where the outstanding command must remain recoverable.
func (s *Store) DiscardPendingResume(sessionID string) error {
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pendingResumes[sessionID]; !exists {
		return nil
	}
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], nil); err != nil {
		return err
	}
	delete(s.pendingResumes, sessionID)
	return nil
}

func (s *Store) retirePendingResume(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingResumes[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return errors.New("pending resume command identity changed")
	}
	if err := s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], nil); err != nil {
		return err
	}
	delete(s.pendingResumes, sessionID)
	return nil
}

// StagePendingRun atomically moves one draft into the durable runtime outbox.
// A crash observes either the editable draft or the replayable command, never
// an ownership gap between separate files.
func (s *Store) StagePendingRun(pending PendingRun) error {
	if pending.State != PendingRunQueued {
		return fmt.Errorf("stage pending run: initial state must be %q", PendingRunQueued)
	}
	if err := pending.validate(pending.Command.SessionID); err != nil {
		return err
	}
	pending = clonePendingRun(pending)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.pendingRuns[pending.Command.SessionID] {
		if current.Command.CommandID == pending.Command.CommandID {
			if !pendingRunEqual(current, pending) {
				return errors.New("pending run command identity already owns another payload")
			}
			return nil
		}
	}
	next := clonePendingRuns(s.pendingRuns)
	next[pending.Command.SessionID] = append(next[pending.Command.SessionID], pending)
	if err := validatePendingRunSequence(pending.Command.SessionID, next[pending.Command.SessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(pending.Command.SessionID, agent.Message{}, next[pending.Command.SessionID]); err != nil {
		return err
	}
	delete(s.drafts, pending.Command.SessionID)
	s.pendingRuns = next
	return nil
}

// SavePendingRuns atomically replaces one session's ordered runtime outbox.
// Queue edits use this boundary so reordering, replacement, and deletion are
// crash-consistent with the next launch.
func (s *Store) SavePendingRuns(sessionID string, commands []PendingRun) error {
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	if err := validatePendingRunSequence(sessionID, commands); err != nil {
		return err
	}
	commands = clonePendingRunSlice(commands)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], commands); err != nil {
		return err
	}
	if len(commands) == 0 {
		delete(s.pendingRuns, sessionID)
	} else {
		s.pendingRuns[sessionID] = commands
	}
	return nil
}

func (s *Store) MarkPendingRunDispatching(
	sessionID string,
	commandID agent.CommandID,
	replay commandreplay.Guard,
) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return errors.New("pending run is absent")
	}
	next := clonePendingRuns(s.pendingRuns)
	if err := next[sessionID][index].beginDispatch(replay); err != nil {
		return err
	}
	if next[sessionID][index].State == s.pendingRuns[sessionID][index].State {
		return nil
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return err
	}
	s.pendingRuns = next
	return nil
}

// MarkPendingRunCanceling durably records that a command with an uncertain
// acknowledgement must be canceled as soon as its run identity is recovered.
func (s *Store) MarkPendingRunCanceling(
	sessionID string,
	commandID agent.CommandID,
	replay commandreplay.Guard,
) (agent.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return "", errors.New("pending run is absent")
	}
	if s.pendingRuns[sessionID][index].State == PendingRunCanceling {
		return s.pendingRuns[sessionID][index].CancelCommandID, nil
	}
	next := clonePendingRuns(s.pendingRuns)
	cancelCommandID, err := next[sessionID][index].beginCancellation(replay, agent.NewCommandID)
	if err != nil {
		return "", err
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return "", err
	}
	s.pendingRuns = next
	return cancelCommandID, nil
}

// RequeuePendingRun turns a definitively refused runtime mutation back into an
// ordinary FIFO entry. A new identity is mandatory: the runtime has already
// bound the old key to its rejection outcome.
func (s *Store) RequeuePendingRun(sessionID string, commandID agent.CommandID) (agent.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return "", errors.New("pending run is absent")
	}
	next := clonePendingRuns(s.pendingRuns)
	replacement, err := next[sessionID][index].requeue(agent.NewCommandID)
	if err != nil {
		return "", err
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return "", err
	}
	s.pendingRuns = next
	return replacement, nil
}

// AcknowledgePendingRun retires only the command the caller actually observed.
// The identity check prevents a late acknowledgement from deleting newer work.
func (s *Store) AcknowledgePendingRun(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := cliidentity.ValidateSession(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	commands := s.pendingRuns[sessionID]
	index := pendingRunIndex(commands, commandID)
	if index < 0 {
		return errors.New("pending run command identity changed")
	}
	if err := commands[index].acknowledgeable(); err != nil {
		return err
	}
	message := commands[index].Command.Message.Clone()
	nextHistory := cloneHistory(s.history)
	historyIndex := slices.IndexFunc(nextHistory, func(entry historyEntry) bool {
		return entry.CommandID == commandID
	})
	if historyIndex >= 0 && !nextHistory[historyIndex].Equal(message) {
		return errors.New("prompt history command identity already owns another message")
	}
	if historyIndex < 0 {
		nextHistory = s.trimHistory(append(nextHistory, historyEntry{Message: message, CommandID: commandID}))
		if err := s.save("history.json", nextHistory); err != nil {
			return err
		}
		// History and the session outbox are separate durable aggregates. Publish
		// the completed first half immediately so a failed outbox replacement can
		// retry by command identity without appending the prompt a second time.
		s.history = nextHistory
	}
	next := clonePendingRuns(s.pendingRuns)
	next[sessionID] = slices.Delete(next[sessionID], index, index+1)
	if len(next[sessionID]) == 0 {
		delete(next, sessionID)
	}
	if err := validatePendingRunSequence(sessionID, next[sessionID]); err != nil {
		return err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next[sessionID]); err != nil {
		return err
	}
	s.pendingRuns = next
	s.history = s.trimHistory(s.history)
	return nil
}
