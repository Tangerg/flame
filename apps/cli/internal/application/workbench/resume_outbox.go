package workbench

import (
	"errors"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

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
	return s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.queue.bySession[sessionID], &pending)
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
	return s.saveSessionStateWithResume(sessionID, s.drafts[sessionID], s.queue.bySession[sessionID], nil)
}
