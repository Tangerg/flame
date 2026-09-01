package workbench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// PendingSteer owns an instruction and its borrowed composer attachments until
// the runtime definitively accepts or rejects the exact command identity.
// Replay metadata binds cold recovery to the runtime idempotency store that
// first received the command.
type PendingSteer struct {
	sessionID string
	command   agent.SteerRun
	stagedAt  time.Time
	replay    commandreplay.Guard
}

type pendingSteerRecord struct {
	SessionID string              `json:"sessionId"`
	Command   agent.SteerRun      `json:"command"`
	StagedAt  time.Time           `json:"stagedAt"`
	Replay    commandreplay.Guard `json:"replay"`
}

const steerSourcePrefix = "/steer "

// NewPendingSteer constructs one complete durable steer ownership fact. The
// instruction must already be canonical so the Runtime command, prompt
// history, and source composer draft cannot represent different text.
func NewPendingSteer(
	sessionID string,
	command agent.SteerRun,
	stagedAt time.Time,
	replay commandreplay.Guard,
) (PendingSteer, error) {
	pending := PendingSteer{
		sessionID: sessionID,
		command:   command.Clone(),
		stagedAt:  stagedAt.UTC(),
		replay:    replay,
	}
	if err := pending.Validate(); err != nil {
		return PendingSteer{}, err
	}
	return pending, nil
}

// Validate enforces the complete persisted command and replay shape.
func (p PendingSteer) Validate() error {
	if err := runtimeprotocol.ValidateSessionID(p.sessionID); err != nil {
		return fmt.Errorf("pending steer: %w", err)
	}
	if err := p.command.Validate(); err != nil {
		return err
	}
	if p.command.CommandID == "" {
		return errors.New("pending steer command id is empty")
	}
	if p.command.Message.Text != strings.TrimSpace(p.command.Message.Text) {
		return errors.New("pending steer instruction must not have surrounding whitespace")
	}
	if p.stagedAt.IsZero() {
		return errors.New("pending steer staging time is empty")
	}
	if p.stagedAt.Location() != time.UTC {
		return errors.New("pending steer staging time must be UTC")
	}
	if err := p.replay.Validate(); err != nil {
		return err
	}
	if p.replay.Protected() && !p.replay.Until().After(p.stagedAt) {
		return errors.New("pending steer replay guarantee is incomplete")
	}
	return nil
}

func (p PendingSteer) validateSession(sessionID string) error {
	if p.sessionID != sessionID {
		return errors.New("pending steer belongs to another session")
	}
	return p.Validate()
}

func (p PendingSteer) clone() PendingSteer {
	p.command = p.command.Clone()
	return p
}

func pendingSteerEqual(left, right PendingSteer) bool {
	return left.sessionID == right.sessionID && left.command.Equal(right.command) &&
		left.stagedAt.Equal(right.stagedAt) && left.replay == right.replay
}

func (p PendingSteer) record() pendingSteerRecord {
	return pendingSteerRecord{
		SessionID: p.sessionID, Command: p.command.Clone(), StagedAt: p.stagedAt, Replay: p.replay,
	}
}

func restorePendingSteer(record pendingSteerRecord) (PendingSteer, error) {
	return NewPendingSteer(record.SessionID, record.Command, record.StagedAt, record.Replay)
}

func (p PendingSteer) SessionID() string           { return p.sessionID }
func (p PendingSteer) CommandID() agent.CommandID  { return p.command.CommandID }
func (p PendingSteer) Command() agent.SteerRun     { return p.command.Clone() }
func (p PendingSteer) Message() agent.Message      { return p.command.Message.Clone() }
func (p PendingSteer) StagedAt() time.Time         { return p.stagedAt }
func (p PendingSteer) Replay() commandreplay.Guard { return p.replay }

// PendingSteers returns unsettled steer commands in stable session order.
func (s *Store) PendingSteers() []PendingSteer {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := make([]PendingSteer, 0, len(s.pendingSteers))
	for _, steer := range s.pendingSteers {
		pending = append(pending, steer.clone())
	}
	slices.SortFunc(pending, func(left, right PendingSteer) int {
		return strings.Compare(left.sessionID, right.sessionID)
	})
	return pending
}

// PendingSteer returns the unsettled command for one session, when present.
func (s *Store) PendingSteer(sessionID string) (PendingSteer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[sessionID]
	return pending.clone(), exists
}

// StagePendingSteer atomically transfers attachment ownership from the exact
// durable composer draft into a replayable runtime command. A crash therefore
// observes either the editable attachments or the command journal, never an
// empty gap between them.
func (s *Store) StagePendingSteer(pending PendingSteer, sourceDraft agent.Message) error {
	pending = pending.clone()
	if err := pending.Validate(); err != nil {
		return err
	}
	sourceDraft = sourceDraft.Clone()
	wantCommand := steerSourcePrefix + pending.command.Message.Text
	if strings.TrimSpace(sourceDraft.Text) != wantCommand {
		return errors.New("pending steer source draft does not contain the exact command")
	}
	if !slices.Equal(sourceDraft.Attachments, pending.command.Message.Attachments) {
		return errors.New("pending steer does not own the source draft attachments")
	}
	if !messageEmpty(sourceDraft) {
		if err := sourceDraft.Validate(); err != nil {
			return fmt.Errorf("pending steer source draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingSteers[pending.sessionID]; exists {
		if pendingSteerEqual(current, pending) {
			return nil
		}
		return errors.New("another steer command is already pending")
	}
	if current, exists := s.drafts[pending.sessionID]; exists != !messageEmpty(sourceDraft) ||
		(exists && !current.Equal(sourceDraft)) {
		return errors.New("session draft changed before steer attachment transfer")
	}
	if err := s.saveSessionStateRecord(
		pending.sessionID, agent.Message{}, s.pendingRuns[pending.sessionID],
		s.pendingResumePointer(pending.sessionID), s.pendingRollbackPointer(pending.sessionID), &pending,
	); err != nil {
		return err
	}
	delete(s.drafts, pending.sessionID)
	s.pendingSteers[pending.sessionID] = pending
	return nil
}

// AcknowledgePendingSteer consumes the exact accepted command, records its
// semantic prompt history idempotently, and preserves any newer session draft.
func (s *Store) AcknowledgePendingSteer(sessionID string, commandID agent.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[sessionID]
	if !exists || pending.command.CommandID != commandID {
		return errors.New("pending steer command identity changed")
	}

	message := pending.command.Message.Clone()
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
		s.history = nextHistory
	}
	if err := s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	); err != nil {
		return err
	}
	delete(s.pendingSteers, sessionID)
	return nil
}

// RejectPendingSteer returns borrowed attachments after a definitive runtime
// refusal. currentDraft is an ownership precondition supplied after the
// terminal has flushed newer input, so the merge and journal retirement are
// one durable session-aggregate replacement.
func (s *Store) RejectPendingSteer(
	sessionID string,
	commandID agent.CommandID,
	currentDraft agent.Message,
) (agent.Message, error) {
	if err := commandID.Validate(); err != nil {
		return agent.Message{}, err
	}
	if err := runtimeprotocol.ValidateSessionID(sessionID); err != nil {
		return agent.Message{}, err
	}
	currentDraft = currentDraft.Clone()
	if !messageEmpty(currentDraft) {
		if err := currentDraft.Validate(); err != nil {
			return agent.Message{}, fmt.Errorf("current session draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pending, exists := s.pendingSteers[sessionID]
	if !exists || pending.command.CommandID != commandID {
		return agent.Message{}, errors.New("pending steer command identity changed")
	}
	if current, present := s.drafts[sessionID]; present != !messageEmpty(currentDraft) ||
		(present && !current.Equal(currentDraft)) {
		return agent.Message{}, errors.New("session draft changed before steer rejection settlement")
	}
	recovered := MergeSteerAttachments(currentDraft, pending.command.Message.Attachments)
	if err := s.saveSessionStateRecord(
		sessionID, recovered, s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	); err != nil {
		return agent.Message{}, err
	}
	if messageEmpty(recovered) {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = recovered.Clone()
	}
	delete(s.pendingSteers, sessionID)
	return recovered, nil
}

// MergeSteerAttachments preserves newer text and attachments while returning
// each rejected attachment at most once.
func MergeSteerAttachments(current agent.Message, rejected []agent.Attachment) agent.Message {
	current = current.Clone()
	seenIDs := make(map[string]struct{}, len(current.Attachments)+len(rejected))
	seenPaths := make(map[string]struct{}, len(current.Attachments)+len(rejected))
	for _, attachment := range current.Attachments {
		seenIDs[attachment.ID] = struct{}{}
		seenPaths[attachment.Path] = struct{}{}
	}
	for _, attachment := range rejected {
		if _, duplicate := seenIDs[attachment.ID]; duplicate {
			continue
		}
		if _, duplicate := seenPaths[attachment.Path]; duplicate {
			continue
		}
		current.Attachments = append(current.Attachments, attachment)
		seenIDs[attachment.ID] = struct{}{}
		seenPaths[attachment.Path] = struct{}{}
	}
	return current
}

func (s *Store) pendingRollbackPointer(sessionID string) *PendingSessionRollback {
	pending, exists := s.pendingRollbacks[sessionID]
	if !exists {
		return nil
	}
	cloned := pending.clone()
	return &cloned
}

func (s *Store) pendingSteerPointer(sessionID string) *PendingSteer {
	pending, exists := s.pendingSteers[sessionID]
	if !exists {
		return nil
	}
	cloned := pending.clone()
	return &cloned
}
