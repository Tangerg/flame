package terminal

import (
	"context"
	"fmt"
	"slices"

	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
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

// A finish event alone cannot prove the steer's Item is absent; only an
// authoritative read of the finished Run can.
type runFinishEvidence uint8

const (
	finishUnseen runFinishEvidence = iota
	finishObserved
	finishReadFailed
	finishRead
)

type steerReceipt struct {
	sessionID   string
	commandID   authoring.CommandID
	runID       string
	receipt     protocol.SteerRunResponse
	userItems   map[string]struct{}
	finish      runFinishEvidence
	readFailure error
	presented   steerReceiptStatus
}

func (s *steerReceipt) observeFinished() {
	if s.finish == finishUnseen {
		s.finish = finishObserved
	}
}

func (s *steerReceipt) readFinished() {
	s.finish, s.readFailure = finishRead, nil
}

func (s *steerReceipt) failRead(err error) {
	s.finish, s.readFailure = finishReadFailed, err
}

func (s *steerReceipt) status() steerReceiptStatus {
	if s.receipt.UserItemID != "" {
		if _, found := s.userItems[s.receipt.UserItemID]; found {
			return steerApplied
		}
		switch s.finish {
		case finishUnseen:
			return steerAccepted
		case finishObserved:
			return steerVerifying
		case finishRead:
			return steerNotApplied
		case finishReadFailed:
		}
	}
	if s.finish != finishUnseen {
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
				entry.readFinished()
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
				entry.observeFinished()
			}
		}
	}
}

func (s *steerReceipts) needingRead(sessionID string) []*steerReceipt {
	var pending []*steerReceipt
	for _, entry := range s.entries {
		if entry.sessionID == sessionID && entry.finish == finishObserved && entry.status() != steerApplied {
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
	s.tasks.session(steerReceiptReadOperation,
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
				if entry.finish == finishRead || entry.status() == steerApplied {
					continue
				}
				failure := err
				if failure == nil {
					failure = fmt.Errorf("authoritative read did not include finished run %s", entry.runID)
				}
				entry.failRead(failure)
			}
			s.presentSteerReceipts()
			s.readSteerReceipts()
		},
	)
}

func (s *steerDesk) presentSteerReceipts() {
	selected := s.receiptToPresent()
	if selected == nil {
		return
	}
	identity := selected.receipt.UserItemID
	if identity == "" {
		identity = string(selected.commandID)
	}
	status := selected.status()
	label := steerReceiptLabel(status)
	if selected.runID != s.runs.conversation().RunID() {
		label += format.Separator + shortIdentity(selected.runID)
	}
	label += format.Separator + identity
	if selected.finish == finishReadFailed && status == steerUnconfirmed {
		label += ": " + selected.readFailure.Error()
	}
	s.notify(label)
}

func (s *steerDesk) receiptToPresent() *steerReceipt {
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
		return nil
	}
	runID := selected.runID
	for _, entry := range s.receipts.entries {
		if entry.sessionID == s.session.current.ID && entry.runID == runID &&
			steerReceiptPriority(entry.status()) >= steerReceiptPriority(selected.status()) {
			selected = entry
		}
	}
	return selected
}

func steerReceiptLabel(status steerReceiptStatus) string {
	switch status {
	case steerAccepted:
		return "steer accepted · waiting to enter model context"
	case steerVerifying:
		return "steer accepted · verifying application"
	case steerApplied:
		return "steer applied to model context"
	case steerNotApplied:
		return "steer not applied to this run"
	case steerUnconfirmed:
		return "steer application remains unconfirmed after this run"
	default:
		return ""
	}
}

// A later group outranks an earlier one when several receipts describe one steer.
var steerReceiptPrecedence = [][]steerReceiptStatus{
	{steerApplied},
	{steerAccepted, steerVerifying},
	{steerNotApplied, steerUnconfirmed},
}

func steerReceiptPriority(status steerReceiptStatus) int {
	return slices.IndexFunc(steerReceiptPrecedence, func(group []steerReceiptStatus) bool { return slices.Contains(group, status) }) + 1
}
