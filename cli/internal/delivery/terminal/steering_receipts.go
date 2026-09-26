package terminal

import (
	"context"
	"fmt"
	"slices"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

type steerReceiptStatus string

const (
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
	commandID         agent.CommandID
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
	return ""
}

func (s *steerReceipt) observeBlock(block agent.Block) {
	if block.RunID != s.runID || block.Kind != agent.BlockUser || block.Status != agent.BlockStatusCompleted {
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

func (s *steerReceipts) observeSnapshot(snapshot agent.SessionSnapshot) {
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

func (s *steerReceipts) observeEvent(sessionID string, envelope agent.RunEvent) {
	for _, entry := range s.entries {
		if entry.sessionID != sessionID || entry.runID != envelope.RunID {
			continue
		}
		switch event := envelope.Event.(type) {
		case agent.BlockCompleted:
			entry.observeBlock(event.Block)
		case agent.RunFinished:
			entry.finishedObserved = true
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

func (a *app) restoreSteerReceipts(snapshot agent.SessionSnapshot) {
	for _, pending := range a.workbench.PendingSteers() {
		a.steers.track(pending)
	}
	a.steers.observeSnapshot(snapshot)
	a.refreshSteerPresentation()
}

func (a *app) refreshSteerPresentation() {
	for _, entry := range a.steers.entries {
		if entry.sessionID == a.session.current.ID && entry.runID == a.execution.conversation.RunID() {
			entry.presented = ""
		}
	}
	a.presentSteerReceipts()
}

func (a *app) observeSteerEvent(event agent.RunEvent) {
	a.steers.observeEvent(a.session.current.ID, event)
	a.presentSteerReceipts()
	a.readSteerReceipts()
}

func (a *app) readSteerReceipts() {
	sessionID := a.session.current.ID
	pending := a.steers.needingRead(sessionID)
	if len(pending) == 0 || a.operations.Active(steerReceiptReadOperation) {
		return
	}
	a.runOperation(steerReceiptReadOperation, false,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			return a.readSessionAfterMutation(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot, err error) {
			if err == nil && snapshot.Session.ID != sessionID {
				err = fmt.Errorf("steer receipt read returned session %s instead of %s", snapshot.Session.ID, sessionID)
			}
			if err == nil {
				a.steers.observeSnapshot(snapshot)
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
			a.presentSteerReceipts()
			a.readSteerReceipts()
		},
	)
}

func (a *app) presentSteerReceipts() {
	var selected *steerReceipt
	for _, entry := range a.steers.entries {
		if entry.sessionID != a.session.current.ID {
			continue
		}
		status := entry.status()
		if status == "" || entry.presented == status {
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
	for _, entry := range a.steers.entries {
		if entry.sessionID == a.session.current.ID && entry.runID == runID &&
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
	if selected.runID != a.execution.conversation.RunID() {
		label += " · " + shortIdentity(selected.runID)
	}
	label += " · " + identity
	if selected.readFailure != nil && status == steerUnconfirmed {
		label += ": " + selected.readFailure.Error()
	}
	a.message(label)
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
