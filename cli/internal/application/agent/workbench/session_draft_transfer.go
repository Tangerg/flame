package workbench

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

const sessionDraftTransferName = "session-draft-transfer.json"

// DraftTransfer is the durable ownership change for composer state that spans
// two session aggregates. Before values fence stale recovery; after values let
// Open finish either half of an interrupted transfer without guessing.
type DraftTransfer struct {
	SourceSessionID      string        `json:"sourceSessionId"`
	DestinationSessionID string        `json:"destinationSessionId"`
	SourceBefore         agent.Message `json:"sourceBefore"`
	SourceAfter          agent.Message `json:"sourceAfter"`
	DestinationBefore    agent.Message `json:"destinationBefore"`
	DestinationAfter     agent.Message `json:"destinationAfter"`
}

func (d DraftTransfer) clone() DraftTransfer {
	d.SourceBefore = d.SourceBefore.Clone()
	d.SourceAfter = d.SourceAfter.Clone()
	d.DestinationBefore = d.DestinationBefore.Clone()
	d.DestinationAfter = d.DestinationAfter.Clone()
	return d
}

func (d DraftTransfer) equal(other DraftTransfer) bool {
	return d.SourceSessionID == other.SourceSessionID &&
		d.DestinationSessionID == other.DestinationSessionID &&
		d.SourceBefore.Equal(other.SourceBefore) &&
		d.SourceAfter.Equal(other.SourceAfter) &&
		d.DestinationBefore.Equal(other.DestinationBefore) &&
		d.DestinationAfter.Equal(other.DestinationAfter)
}

func (d DraftTransfer) validate() error {
	if err := runtimeprotocol.ValidateSessionID(d.SourceSessionID); err != nil {
		return fmt.Errorf("draft transfer source: %w", err)
	}
	if err := runtimeprotocol.ValidateSessionID(d.DestinationSessionID); err != nil {
		return fmt.Errorf("draft transfer destination: %w", err)
	}
	if d.SourceSessionID == d.DestinationSessionID {
		return errors.New("draft transfer source and destination are the same session")
	}
	values := []struct {
		label   string
		message agent.Message
	}{
		{label: "source before", message: d.SourceBefore},
		{label: "source after", message: d.SourceAfter},
		{label: "destination before", message: d.DestinationBefore},
		{label: "destination after", message: d.DestinationAfter},
	}
	for _, value := range values {
		if value.message.IsEmpty() {
			continue
		}
		if err := value.message.Validate(); err != nil {
			return fmt.Errorf("draft transfer %s: %w", value.label, err)
		}
	}
	if d.SourceBefore.Equal(d.SourceAfter) &&
		d.DestinationBefore.Equal(d.DestinationAfter) {
		return errors.New("draft transfer does not change authoring state")
	}
	return nil
}

func (d DraftTransfer) blocks(sessionID string) bool {
	return sessionID == d.SourceSessionID || sessionID == d.DestinationSessionID
}

// MergeSessionDraft combines authoring values when input written during a
// Session transition follows the user into a destination that already owns a
// draft. Existing destination text comes first; incoming text follows as a new
// paragraph. Exact attachment identities are kept once, while conflicting
// identities fail closed instead of silently choosing one user's file value.
func MergeSessionDraft(existing, incoming agent.Message) (agent.Message, error) {
	values := []struct {
		label   string
		message agent.Message
	}{
		{label: "existing", message: existing},
		{label: "incoming", message: incoming},
	}
	for _, value := range values {
		if value.message.IsEmpty() {
			continue
		}
		if err := value.message.Validate(); err != nil {
			return agent.Message{}, fmt.Errorf("%s session draft: %w", value.label, err)
		}
	}
	if existing.IsEmpty() {
		return incoming.Clone(), nil
	}
	if incoming.IsEmpty() {
		return existing.Clone(), nil
	}

	merged := existing.Clone()
	switch {
	case strings.TrimSpace(merged.Text) == "":
		merged.Text = incoming.Text
	case strings.TrimSpace(incoming.Text) == "", merged.Text == incoming.Text:
	default:
		merged.Text += "\n\n" + incoming.Text
	}
	byID := make(map[string]agent.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	byPath := make(map[string]agent.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	for _, item := range merged.Attachments {
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	for _, item := range incoming.Attachments {
		if current, found := byID[item.ID]; found {
			if current != item {
				return agent.Message{}, fmt.Errorf("session drafts conflict on attachment id %q", item.ID)
			}
			continue
		}
		if current, found := byPath[item.Path]; found {
			if current != item {
				return agent.Message{}, fmt.Errorf("session drafts conflict on attachment path %q", item.Path)
			}
			continue
		}
		merged.Attachments = append(merged.Attachments, item)
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	if err := merged.Validate(); err != nil {
		return agent.Message{}, fmt.Errorf("merged session draft: %w", err)
	}
	return merged, nil
}

// ApplyDraftTransfer changes both session drafts under one restart-safe
// journal. If a filesystem failure leaves only one side committed, subsequent
// mutations of either aggregate fail closed until Open completes the journal.
func (s *Store) ApplyDraftTransfer(transfer DraftTransfer) error {
	transfer = transfer.clone()
	if err := transfer.validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftTransfer != nil {
		if !s.draftTransfer.equal(transfer) {
			return errors.New("another session draft transfer requires recovery")
		}
		return s.completeDraftTransferLocked()
	}
	if err := transfer.expect(s.drafts); err != nil {
		return err
	}
	if err := s.save(sessionDraftTransferName, transfer); err != nil {
		return fmt.Errorf("stage session draft transfer: %w", err)
	}
	s.draftTransfer = &transfer
	return s.completeDraftTransferLocked()
}

func (d DraftTransfer) expect(drafts map[string]agent.Message) error {
	if !drafts[d.SourceSessionID].Equal(d.SourceBefore) {
		return errors.New("source draft changed before session transfer")
	}
	if !drafts[d.DestinationSessionID].Equal(d.DestinationBefore) {
		return errors.New("destination draft changed before session transfer")
	}
	return nil
}

func (s *Store) recoverDraftTransfer() error {
	if s.draftTransfer == nil {
		return nil
	}
	if err := s.draftTransfer.validate(); err != nil {
		return err
	}
	return s.completeDraftTransferLocked()
}

func (s *Store) completeDraftTransferLocked() error {
	transfer := s.draftTransfer
	if transfer == nil {
		return nil
	}
	if err := validateTransferSide(
		"source", s.drafts[transfer.SourceSessionID], transfer.SourceBefore, transfer.SourceAfter,
	); err != nil {
		return err
	}
	if err := validateTransferSide(
		"destination", s.drafts[transfer.DestinationSessionID],
		transfer.DestinationBefore, transfer.DestinationAfter,
	); err != nil {
		return err
	}
	// Materialize the destination first so the journal and at least one durable
	// aggregate always retain a transferred draft before its source is retired.
	if err := s.advanceDraftTransferSide(
		transfer.DestinationSessionID, transfer.DestinationBefore, transfer.DestinationAfter,
	); err != nil {
		return fmt.Errorf("save destination draft transfer: %w", err)
	}
	if err := s.advanceDraftTransferSide(
		transfer.SourceSessionID, transfer.SourceBefore, transfer.SourceAfter,
	); err != nil {
		return fmt.Errorf("save source draft transfer: %w", err)
	}
	if err := s.remove(sessionDraftTransferName); err != nil {
		return fmt.Errorf("retire session draft transfer: %w", err)
	}
	s.draftTransfer = nil
	return nil
}

func validateTransferSide(label string, current, before, after agent.Message) error {
	if current.Equal(before) || current.Equal(after) {
		return nil
	}
	return fmt.Errorf("%s draft changed while session transfer was pending", label)
}

func (s *Store) advanceDraftTransferSide(sessionID string, before, after agent.Message) error {
	current := s.drafts[sessionID]
	if current.Equal(after) {
		return nil
	}
	if !current.Equal(before) {
		return errors.New("draft no longer matches its transfer precondition")
	}
	if err := s.saveSessionStateRecordUnfenced(
		sessionID, after, s.pendingRuns[sessionID], s.pendingResumePointer(sessionID),
		s.pendingRollbackPointer(sessionID), s.pendingSteerPointer(sessionID),
	); err != nil {
		return err
	}
	if after.IsEmpty() {
		delete(s.drafts, sessionID)
	} else {
		s.drafts[sessionID] = after.Clone()
	}
	return nil
}

func (s *Store) draftTransferBlocks(sessionID string) bool {
	return s.draftTransfer != nil && s.draftTransfer.blocks(sessionID)
}
