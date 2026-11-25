package workbench

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type PendingSessionRollback struct {
	CommandID             authoring.CommandID   `json:"commandId"`
	SessionID             string                `json:"sessionId"`
	ToRunID               string                `json:"toRunId,omitzero"`
	Scope                 protocol.RestoreType  `json:"scope"`
	ExpectedLastRootRunID string                `json:"expectedLastRootRunId"`
	ExpectedWorkspacePath string                `json:"expectedWorkspacePath"`
	StagedAt              time.Time             `json:"stagedAt"`
	Replay                authoring.ReplayGuard `json:"replay"`
	Restoration           *RollbackDraft        `json:"restoration,omitzero"`
}

type RollbackDraft struct {
	DroppedCount  int    `json:"droppedCount"`
	OpeningText   string `json:"openingText,omitzero"`
	OpeningImages int    `json:"openingImages,omitzero"`
}

func (p PendingSessionRollback) Validate() error {
	request := p.Request()
	if err := request.Validate(); err != nil {
		return err
	}
	if request.CommandID == "" {
		return errors.New("session rollback command id is empty")
	}
	if p.StagedAt.IsZero() {
		return errors.New("session rollback staging time is empty")
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if !p.Replay.Protected() || !p.Replay.Until().After(p.StagedAt) {
		return errors.New("session rollback replay guarantee is incomplete")
	}
	if value := p.Restoration; value != nil {
		if value.DroppedCount < 0 || value.OpeningImages < 0 ||
			(request.FilesOnly() && value.DroppedCount != 0) ||
			(value.DroppedCount == 0 && (value.OpeningText != "" || value.OpeningImages != 0)) {
			return errors.New("session rollback restoration contradicts the committed scope")
		}
	}
	return nil
}

func (p PendingSessionRollback) Request() conversation.RollbackSession {
	return conversation.RollbackSession{
		RollbackTarget:        conversation.RollbackTarget{SessionID: p.SessionID, ToRunID: p.ToRunID, Scope: p.Scope},
		CommandID:             p.CommandID,
		ExpectedLastRootRunID: p.ExpectedLastRootRunID,
		ExpectedWorkspacePath: p.ExpectedWorkspacePath,
	}
}

func (p PendingSessionRollback) clone() PendingSessionRollback {
	if p.Restoration != nil {
		p.Restoration = new(*p.Restoration)
	}
	return p
}

func pendingSessionRollbackEqual(left, right PendingSessionRollback) bool {
	return left.Restoration == nil && right.Restoration == nil &&
		left.Request() == right.Request() && left.StagedAt.Equal(right.StagedAt) && left.Replay == right.Replay
}

type SessionRollbackRecovery struct {
	DroppedCount  int
	OpeningImages int
	Merged        bool
}

type SessionDraftActivation struct {
	Draft    authoring.Message
	Rollback *SessionRollbackRecovery
}

func (s *Store) PendingSessionRollbacks() []PendingSessionRollback {
	s.mu.Lock()
	defer s.mu.Unlock()
	rollbacks := make([]PendingSessionRollback, 0, len(s.pendingRollbacks))
	for _, pending := range s.pendingRollbacks {
		rollbacks = append(rollbacks, pending.clone())
	}
	slices.SortFunc(rollbacks, func(left, right PendingSessionRollback) int {
		return strings.Compare(left.SessionID, right.SessionID)
	})
	return rollbacks
}

func (s *Store) PendingSessionRollback(sessionID string) (PendingSessionRollback, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	return pending.clone(), exists
}

func (s *Store) StageSessionRollback(pending PendingSessionRollback) error {
	if err := pending.Validate(); err != nil {
		return err
	}
	if pending.Restoration != nil {
		return errors.New("a new session rollback already carries a restoration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingRollbacks[pending.SessionID]; exists {
		if pendingSessionRollbackEqual(current, pending) {
			return nil
		}
		return errors.New("another session rollback is already pending")
	}
	return s.saveSessionStateRecord(
		pending.SessionID, s.drafts[pending.SessionID], s.queue.bySession[pending.SessionID],
		s.pendingResumePointer(pending.SessionID), &pending, s.pendingSteerPointer(pending.SessionID),
	)
}

func (s *Store) ConfirmSessionRollback(sessionID string, commandID authoring.CommandID, restoration RollbackDraft) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists || pending.CommandID != commandID {
		return errors.New("session rollback journal does not match the confirmed command")
	}
	if pending.Restoration != nil {
		if *pending.Restoration != restoration {
			return errors.New("session rollback already carries another restoration")
		}
		return nil
	}
	pending.Restoration = &restoration
	if err := pending.Validate(); err != nil {
		return err
	}
	return s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.queue.bySession[sessionID],
		s.pendingResumePointer(sessionID), &pending, s.pendingSteerPointer(sessionID),
	)
}

func (s *Store) RejectSessionRollback(sessionID string, commandID authoring.CommandID) error {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists {
		return nil
	}
	if pending.Restoration != nil || pending.CommandID != commandID {
		return errors.New("session rollback journal does not match the rejected command")
	}
	return s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.queue.bySession[sessionID],
		s.pendingResumePointer(sessionID), nil, s.pendingSteerPointer(sessionID),
	)
}

func (s *Store) ActivateSessionDraft(sessionID string, incoming authoring.Message) (SessionDraftActivation, error) {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return SessionDraftActivation{}, err
	}
	incoming = incoming.Clone()
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.drafts[sessionID]
	draft, err := MergeSessionDraft(current, incoming)
	if err != nil {
		return SessionDraftActivation{}, err
	}
	pending, exists := s.pendingRollbacks[sessionID]
	confirmed := exists && pending.Restoration != nil
	var recovery *SessionRollbackRecovery
	if confirmed {
		var merged bool
		draft, merged = mergeRollbackText(draft, pending.Restoration.OpeningText)
		recovery = &SessionRollbackRecovery{
			DroppedCount:  pending.Restoration.DroppedCount,
			OpeningImages: pending.Restoration.OpeningImages,
			Merged:        merged,
		}
	}
	if current.Equal(draft) && !confirmed {
		return SessionDraftActivation{Draft: draft.Clone()}, nil
	}
	var rollback *PendingSessionRollback
	if exists && !confirmed {
		cloned := pending.clone()
		rollback = &cloned
	}
	if err := s.saveSessionStateRecord(
		sessionID, draft, s.queue.bySession[sessionID], s.pendingResumePointer(sessionID), rollback,
		s.pendingSteerPointer(sessionID),
	); err != nil {
		return SessionDraftActivation{}, err
	}
	return SessionDraftActivation{Draft: draft.Clone(), Rollback: recovery}, nil
}

func (s *Store) pendingResumePointer(sessionID string) *PendingResume {
	pending, exists := s.pendingResumes[sessionID]
	if !exists {
		return nil
	}
	return &pending
}

func mergeRollbackText(current authoring.Message, text string) (authoring.Message, bool) {
	recovered := authoring.Message{Text: text}
	current = current.Clone()
	recovered = recovered.Clone()
	if recovered.IsEmpty() {
		return current, false
	}
	if current.IsEmpty() {
		return recovered, false
	}
	if current.Equal(recovered) ||
		(strings.TrimSpace(recovered.Text) != "" && strings.HasPrefix(current.Text, recovered.Text)) {
		return current, false
	}
	if strings.TrimSpace(recovered.Text) != "" {
		if strings.TrimSpace(current.Text) == "" {
			current.Text = recovered.Text
		} else {
			current.Text = recovered.Text + "\n\n" + current.Text
		}
	}
	return current, true
}
