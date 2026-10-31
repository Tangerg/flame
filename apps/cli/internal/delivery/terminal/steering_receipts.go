package terminal

import (
	"context"
	"fmt"
	"slices"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type steerReceiptStatus string

const (
	steerPending     steerReceiptStatus = ""
	steerAccepted    steerReceiptStatus = "accepted"
	steerVerifying   steerReceiptStatus = "verifying"
	steerApplied     steerReceiptStatus = "applied"
	steerNotApplied  steerReceiptStatus = "not-applied"
	steerUnconfirmed steerReceiptStatus = "unconfirmed"
)

// These are observations for one locally issued command, not execution state.
// Item and terminal-read evidence may both arrive before the acknowledgement.
type steerReceipt struct {
	sessionID         string
	commandID         authoring.CommandID
	runID             string
	receipt           protocol.SteerRunResponse
	userItems         map[string]struct{}
	finishedObserved  bool
	afterFinishedRead bool
	readFailure       error
	presented         steerReceiptStatus
}

func (s *steerReceipt) status() steerReceiptStatus {
	if s.receipt.UserItemID != "" {
		if _, found := s.userItems[s.receipt.UserItemID]; found {
			return steerApplied
		}
		if s.afterFinishedRead {
			return steerNotApplied
		}
		if s.readFailure == nil {
			if s.finishedObserved {
				return steerVerifying
			}
			return steerAccepted
		}
	}
	if s.finishedObserved || s.afterFinishedRead || s.readFailure != nil {
		return steerUnconfirmed
	}
	return steerPending
}

func (s *steerReceipt) observeBlock(block conversation.Block) {
	if block.RunID != s.runID || block.Kind != conversation.BlockUser || block.Status != conversation.BlockStatusCompleted {
		return
	}
	if s.receipt.UserItemID == "" || block.ID == s.receipt.UserItemID {
		s.userItems[block.ID] = struct{}{}
	}
}

type steerReceipts struct {
	entries []*steerReceipt
}

func (s *steerReceipts) track(pending workbench.PendingSteer) *steerReceipt {
	for _, entry := range s.entries {
		if entry.sessionID == pending.SessionID() && entry.commandID == pending.CommandID() {
			return entry
		}
	}
	command := pending.Command()
	entry := &steerReceipt{
		sessionID: pending.SessionID(), commandID: pending.CommandID(),
		runID: command.RunID, userItems: make(map[string]struct{}),
	}
	s.entries = append(s.entries, entry)
	return entry
}

func (s *steerReceipts) accept(result runworkflow.SteerResult) {
	s.track(result.Pending).receipt = result.Receipt
}

func (s *steerReceipts) reject(pending workbench.PendingSteer) {
	s.entries = slices.DeleteFunc(s.entries, func(entry *steerReceipt) bool {
		return entry.sessionID == pending.SessionID() && entry.commandID == pending.CommandID()
	})
}

func (s *steerReceipts) observeSnapshot(snapshot conversation.SessionSnapshot) {
	for _, entry := range s.entries {
		if entry.sessionID != snapshot.Session.ID {
			continue
		}
		for _, block := range snapshot.Transcript {
			entry.observeBlock(block)
		}
		for _, run := range snapshot.Runs {
			if run.ID == entry.runID && run.Status == protocol.RunStatusFinished {
				entry.afterFinishedRead = true
				entry.readFailure = nil
				break
			}
		}
	}
}

func (s *steerReceipts) observeEvent(sessionID string, envelope conversation.RunEvent) {
	for _, entry := range s.entries {
		if entry.sessionID != sessionID || entry.runID != envelope.RunID {
			continue
		}
		switch event := envelope.Event.(type) {
		case conversation.BlockCompleted:
			entry.observeBlock(event.Block)
		case conversation.SegmentFinished:
			if event.Run.Status == protocol.RunStatusFinished {
				entry.finishedObserved = true
			}
		}
	}
}

func (s *steerReceipts) needingRead(sessionID string) []*steerReceipt {
	var pending []*steerReceipt
	for _, entry := range s.entries {
		if entry.sessionID == sessionID && entry.finishedObserved && !entry.afterFinishedRead &&
			entry.readFailure == nil && entry.status() != steerApplied {
			pending = append(pending, entry)
		}
	}
	return pending
}

func (s *steerDesk) restoreSteerReceipts(snapshot conversation.SessionSnapshot) {
	for _, pending := range s.workbench.PendingSteers() {
		s.receipts.track(pending)
	}
	s.receipts.observeSnapshot(snapshot)
	s.refreshSteerPresentation()
}

func (s *steerDesk) refreshSteerPresentation() {
	for _, entry := range s.receipts.entries {
		if entry.sessionID == s.session.current.ID && entry.runID == s.runs.conversation().RunID() {
			entry.presented = steerPending
		}
	}
	s.presentSteerReceipts()
}

func (s *steerDesk) observeSteerEvent(event conversation.RunEvent) {
	s.receipts.observeEvent(s.session.current.ID, event)
	s.presentSteerReceipts()
	s.readSteerReceipts()
}

func (s *steerDesk) readSteerReceipts() {
	sessionID := s.session.current.ID
	pending := s.receipts.needingRead(sessionID)
	if len(pending) == 0 || s.tasks.operations.Active(steerReceiptReadOperation) {
		return
	}
	s.tasks.session(steerReceiptReadOperation, false,
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			return s.readSession(ctx, sessionID)
		},
		func(snapshot conversation.SessionSnapshot, err error) {
			if err == nil && snapshot.Session.ID != sessionID {
				err = fmt.Errorf("steer receipt read returned session %s instead of %s", snapshot.Session.ID, sessionID)
			}
			if err == nil {
				s.receipts.observeSnapshot(snapshot)
			}
			for _, entry := range pending {
				if entry.afterFinishedRead || entry.status() == steerApplied {
					continue
				}
				entry.readFailure = err
				if entry.readFailure == nil {
					entry.readFailure = fmt.Errorf("authoritative read did not include finished run %s", entry.runID)
				}
			}
			s.presentSteerReceipts()
			s.readSteerReceipts()
		},
	)
}

func (s *steerDesk) presentSteerReceipts() {
	var selected *steerReceipt
	for _, entry := range s.receipts.entries {
		if entry.sessionID != s.session.current.ID {
			continue
		}
		status := entry.status()
		if status == steerPending || entry.presented == status {
			continue
		}
		entry.presented = status
		if selected == nil || steerReceiptPriority(status) >= steerReceiptPriority(selected.status()) {
			selected = entry
		}
	}
	if selected == nil {
		return
	}
	runID := selected.runID
	for _, entry := range s.receipts.entries {
		if entry.sessionID == s.session.current.ID && entry.runID == runID &&
			steerReceiptPriority(entry.status()) >= steerReceiptPriority(selected.status()) {
			selected = entry
		}
	}
	identity := selected.receipt.UserItemID
	if identity == "" {
		identity = string(selected.commandID)
	}
	status := selected.status()
	var label string
	switch status {
	case steerAccepted:
		label = "steer accepted · waiting to enter model context"
	case steerVerifying:
		label = "steer accepted · verifying application"
	case steerApplied:
		label = "steer applied to model context"
	case steerNotApplied:
		label = "steer not applied to this run"
	case steerUnconfirmed:
		label = "steer application remains unconfirmed after this run"
	}
	if selected.runID != s.runs.conversation().RunID() {
		label += " · " + shortIdentity(selected.runID)
	}
	label += " · " + identity
	if selected.readFailure != nil && status == steerUnconfirmed {
		label += ": " + selected.readFailure.Error()
	}
	s.notify(label)
}

func steerReceiptPriority(status steerReceiptStatus) int {
	switch status {
	case steerNotApplied, steerUnconfirmed:
		return 3
	case steerAccepted, steerVerifying:
		return 2
	case steerApplied:
		return 1
	default:
		return 0
	}
}
