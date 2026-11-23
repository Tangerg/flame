package workbench

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (s *Store) PendingRuns(sessionID string) []PendingRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePendingRunSlice(s.pendingRuns[sessionID])
}

func (s *Store) PendingResume(sessionID string) (PendingResume, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingResumes[sessionID]
	return clonePendingResume(pending), ok
}

func (s *Store) StagePendingResume(sessionID string, pending PendingResume, input *PreparedInput) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := pending.validate(); err != nil {
		return err
	}
	pending = clonePendingResume(pending)
	if pending.Command.Message != nil {
		blocks, digest, err := input.bind(s, *pending.Command.Message)
		if err != nil {
			return err
		}
		pending.Command.Input, pending.InputDigest = blocks, digest
	} else if input != nil {
		return errors.New("resume without a message cannot own prepared input")
	}
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
	return s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], &pending)
}

func (s *Store) claimPendingResumeLocked(sessionID string, commandID authoring.CommandID) (PendingResume, error) {
	pending, exists := s.pendingResumes[sessionID]
	if !exists || pending.Command.CommandID != commandID {
		return PendingResume{}, errors.New("pending resume command identity changed")
	}
	return pending, nil
}

func (s *Store) RetirePendingResume(sessionID string, commandID authoring.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.claimPendingResumeLocked(sessionID, commandID); err != nil {
		return err
	}
	return s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.pendingRuns[sessionID], nil)
}

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
	next := append(clonePendingRunSlice(s.pendingRuns[pending.Command.SessionID]), pending)
	if err := validatePendingRunSequence(pending.Command.SessionID, next); err != nil {
		return err
	}
	return s.saveSessionState(pending.Command.SessionID, authoring.Message{}, next)
}

func (s *Store) SavePendingRuns(sessionID string, commands []PendingRun) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := validatePendingRunSequence(sessionID, commands); err != nil {
		return err
	}
	commands = clonePendingRunSlice(commands)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.pendingRuns[sessionID] {
		if current.State == PendingRunQueued {
			continue
		}
		index := pendingRunIndex(commands, current.Command.CommandID)
		if index < 0 || !pendingRunEqual(current, commands[index]) || current.InputDigest != commands[index].InputDigest {
			return errors.New("queue edit cannot replace a dispatched command")
		}
	}
	for _, pending := range commands {
		if pending.State == PendingRunQueued {
			continue
		}
		index := pendingRunIndex(s.pendingRuns[sessionID], pending.Command.CommandID)
		if index < 0 || s.pendingRuns[sessionID][index].State == PendingRunQueued {
			return errors.New("queue edit cannot dispatch a command")
		}
	}
	return s.saveSessionState(sessionID, s.drafts[sessionID], commands)
}

func (s *Store) MarkPendingRunDispatching(
	sessionID string,
	commandID authoring.CommandID,
	replayGuard authoring.ReplayGuard,
	input *PreparedInput,
) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.pendingRunLocked(sessionID, commandID)
	if err != nil {
		return err
	}
	current := s.pendingRuns[sessionID][index]
	if current.State != PendingRunQueued {
		_, err := current.ReplayCommand()
		return err
	}
	blocks, digest, err := input.bind(s, current.Command.Message)
	if err != nil {
		return err
	}
	next := clonePendingRunSlice(s.pendingRuns[sessionID])
	next[index].Command.Input = blocks
	next[index].InputDigest = digest
	dispatching, err := next[index].beginDispatch(replayGuard)
	if err != nil {
		return err
	}
	next[index] = dispatching
	if next[index].State == s.pendingRuns[sessionID][index].State {
		return nil
	}
	if err := validatePendingRunSequence(sessionID, next); err != nil {
		return err
	}
	return s.saveSessionState(sessionID, s.drafts[sessionID], next)
}

func (s *Store) MarkPendingRunCanceling(
	sessionID string,
	commandID authoring.CommandID,
	replayGuard authoring.ReplayGuard,
) (authoring.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.pendingRunLocked(sessionID, commandID)
	if err != nil {
		return "", err
	}
	if s.pendingRuns[sessionID][index].State == PendingRunCanceling {
		return s.pendingRuns[sessionID][index].CancelCommandID, nil
	}
	next := clonePendingRunSlice(s.pendingRuns[sessionID])
	canceling, err := next[index].beginCancellation(replayGuard)
	if err != nil {
		return "", err
	}
	next[index] = canceling
	cancelCommandID := canceling.CancelCommandID
	if err := validatePendingRunSequence(sessionID, next); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next); err != nil {
		return "", err
	}

	return cancelCommandID, nil
}

func (s *Store) RequeuePendingRun(sessionID string, commandID authoring.CommandID) (authoring.CommandID, error) {
	if err := commandID.Validate(); err != nil {
		return "", err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.pendingRunLocked(sessionID, commandID)
	if err != nil {
		return "", err
	}
	next := clonePendingRunSlice(s.pendingRuns[sessionID])
	requeued, err := next[index].requeue()
	if err != nil {
		return "", err
	}
	next[index] = requeued
	replacement := requeued.Command.CommandID
	if err := validatePendingRunSequence(sessionID, next); err != nil {
		return "", err
	}
	if err := s.saveSessionState(sessionID, s.drafts[sessionID], next); err != nil {
		if writeCommitted(err) {
			return replacement, err
		}
		return "", err
	}

	return replacement, nil
}

func (s *Store) AcknowledgePendingRun(sessionID string, commandID authoring.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
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
	if err := s.recordPromptHistoryLocked(commandID, commands[index].Command.Message.Clone()); err != nil {
		return err
	}
	next := clonePendingRunSlice(s.pendingRuns[sessionID])
	next = slices.Delete(next, index, index+1)
	if err := validatePendingRunSequence(sessionID, next); err != nil {
		return err
	}
	return s.saveSessionState(sessionID, s.drafts[sessionID], next)
}

var errPendingRunAbsent = errors.New("pending run is absent")

func (s *Store) pendingRunLocked(sessionID string, commandID authoring.CommandID) (int, error) {
	index := pendingRunIndex(s.pendingRuns[sessionID], commandID)
	if index < 0 {
		return -1, errPendingRunAbsent
	}
	return index, nil
}
