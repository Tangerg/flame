package workbench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// PendingSteer owns an instruction and its borrowed composer attachments until
// the runtime definitively accepts or rejects the exact command identity.
// Replay metadata binds cold recovery to the runtime idempotency store that
// first received the command.
type PendingSteer struct {
	inputDigest  inputDigest
	inputFailure error
	sessionID    string
	command      authoring.SteerRun
	stagedAt     time.Time
	replay       authoring.ReplayGuard
}

type pendingSteerRecord struct {
	InputDigest inputDigest           `json:"inputDigest,omitzero"`
	SessionID   string                `json:"sessionId"`
	Command     authoring.SteerRun    `json:"command"`
	StagedAt    time.Time             `json:"stagedAt"`
	Replay      authoring.ReplayGuard `json:"replay"`
}

// SteerSourcePrefix marks a draft as a steer command. The terminal builds the draft
// with it and StagePendingSteer validates the exact command against it, so the two
// must share one owner.
const SteerSourcePrefix = "/steer "

// NewPendingSteer constructs one complete durable steer ownership fact. The
// instruction must already be canonical so the Runtime command, prompt
// history, and source composer draft cannot represent different text.
func NewPendingSteer(
	sessionID string,
	command authoring.SteerRun,
	stagedAt time.Time,
	replayGuard authoring.ReplayGuard,
) (PendingSteer, error) {
	pending := PendingSteer{
		sessionID: sessionID,
		command:   command.Clone(),
		stagedAt:  stagedAt.UTC(),
		replay:    replayGuard,
	}
	if err := pending.Validate(); err != nil {
		return PendingSteer{}, err
	}
	return pending, nil
}

// Validate enforces the complete persisted command and replay shape.
func (p PendingSteer) Validate() error {
	if err := p.inputDigest.validate(); err != nil {
		return err
	}
	if err := protocol.ValidateSessionID(p.sessionID); err != nil {
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
		InputDigest: p.inputDigest,
		SessionID:   p.sessionID, Command: p.command.Clone(), StagedAt: p.stagedAt, Replay: p.replay,
	}
}

func restorePendingSteer(record pendingSteerRecord) (PendingSteer, error) {
	pending, err := NewPendingSteer(record.SessionID, record.Command, record.StagedAt, record.Replay)
	pending.inputDigest = record.InputDigest
	return pending, err
}

func (p PendingSteer) SessionID() string              { return p.sessionID }
func (p PendingSteer) CommandID() authoring.CommandID { return p.command.CommandID }
func (p PendingSteer) Command() authoring.SteerRun    { return p.command.Clone() }
func (p PendingSteer) Message() authoring.Message     { return p.command.Message.Clone() }
func (p PendingSteer) StagedAt() time.Time            { return p.stagedAt }
func (p PendingSteer) Replay() authoring.ReplayGuard  { return p.replay }

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
func (s *Store) StagePendingSteer(pending PendingSteer, sourceDraft authoring.Message, input *PreparedInput) (PendingSteer, error) {
	pending = pending.clone()
	blocks, digest, err := input.bind(s, pending.command.Message)
	if err != nil {
		return PendingSteer{}, err
	}
	pending.command.Input, pending.inputDigest = blocks, digest
	sourceDraft = sourceDraft.Clone()
	wantCommand := SteerSourcePrefix + pending.command.Message.Text
	if strings.TrimSpace(sourceDraft.Text) != wantCommand {
		return PendingSteer{}, errors.New("pending steer source draft does not contain the exact command")
	}
	if !slices.Equal(sourceDraft.Attachments, pending.command.Message.Attachments) {
		return PendingSteer{}, errors.New("pending steer does not own the source draft attachments")
	}
	if !sourceDraft.IsEmpty() {
		if err := sourceDraft.Validate(); err != nil {
			return PendingSteer{}, fmt.Errorf("pending steer source draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.pendingSteers[pending.sessionID]; exists {
		if pendingSteerEqual(current, pending) {
			return current.clone(), nil
		}
		return PendingSteer{}, errors.New("another steer command is already pending")
	}
	if current, exists := s.drafts[pending.sessionID]; exists != !sourceDraft.IsEmpty() ||
		(exists && !current.Equal(sourceDraft)) {
		return PendingSteer{}, errors.New("session draft changed before steer attachment transfer")
	}
	if err := s.saveSessionStateRecord(
		pending.sessionID, authoring.Message{}, s.pendingRuns[pending.sessionID],
		s.pendingResumePointer(pending.sessionID), s.pendingRollbackPointer(pending.sessionID), &pending,
	); err != nil {
		return PendingSteer{}, err
	}

	return pending.clone(), nil
}

// AcknowledgePendingSteer consumes the exact accepted command, records its
// semantic prompt history idempotently, and preserves any newer session draft.
// claimPendingSteerLocked returns the steer a settlement names. Acknowledgement
// and rejection are the same claim asked twice, so they answer with one spelling
// rather than each deciding for itself which steer it is finishing.
func (s *Store) claimPendingSteerLocked(sessionID string, commandID authoring.CommandID) (PendingSteer, error) {
	pending, exists := s.pendingSteers[sessionID]
	if !exists || pending.command.CommandID != commandID {
		return PendingSteer{}, errors.New("pending steer command identity changed")
	}
	return pending, nil
}

func (s *Store) AcknowledgePendingSteer(sessionID string, commandID authoring.CommandID) error {
	if err := commandID.Validate(); err != nil {
		return err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.claimPendingSteerLocked(sessionID, commandID)
	if err != nil {
		return err
	}

	if err := s.recordPromptHistoryLocked(commandID, pending.command.Message.Clone()); err != nil {
		return err
	}
	return s.saveSessionStateRecord(
		sessionID, s.drafts[sessionID], s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	)
}

// RejectPendingSteer returns borrowed attachments after a definitive runtime
// refusal. currentDraft is an ownership precondition supplied after the
// terminal has flushed newer input, so the merge and journal retirement are
// one durable session-aggregate replacement.
func (s *Store) RejectPendingSteer(
	sessionID string,
	commandID authoring.CommandID,
	currentDraft authoring.Message,
) (authoring.Message, error) {
	if err := commandID.Validate(); err != nil {
		return authoring.Message{}, err
	}
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return authoring.Message{}, err
	}
	currentDraft = currentDraft.Clone()
	if !currentDraft.IsEmpty() {
		if err := currentDraft.Validate(); err != nil {
			return authoring.Message{}, fmt.Errorf("current session draft: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.claimPendingSteerLocked(sessionID, commandID)
	if err != nil {
		return authoring.Message{}, err
	}
	if current, present := s.drafts[sessionID]; present != !currentDraft.IsEmpty() ||
		(present && !current.Equal(currentDraft)) {
		return authoring.Message{}, errors.New("session draft changed before steer rejection settlement")
	}
	recovered := MergeSteerAttachments(currentDraft, pending.command.Message.Attachments)
	if err := s.saveSessionStateRecord(
		sessionID, recovered, s.pendingRuns[sessionID],
		s.pendingResumePointer(sessionID), s.pendingRollbackPointer(sessionID), nil,
	); err != nil {
		return authoring.Message{}, err
	}
	return recovered, nil
}

// MergeSteerAttachments preserves newer text and attachments while returning
// each rejected attachment at most once.
func MergeSteerAttachments(current authoring.Message, rejected []authoring.Attachment) authoring.Message {
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
