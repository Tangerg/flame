package workbench

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type promptClaim uint8

const (
	unclaimedPrompt promptClaim = iota
	editingPrompt
	preparingPrompt
)

type PendingRun struct {
	EntryID  authoring.QueueEntryID `json:"entryId"`
	Command  authoring.StartRun     `json:"command"`
	Dispatch *RunDispatch           `json:"dispatch,omitzero"`
	claim    promptClaim
}

type RunDispatch struct {
	InputDigest  inputDigest           `json:"inputDigest,omitzero"`
	Replay       authoring.ReplayGuard `json:"replay"`
	Cancellation *StartCancellation    `json:"cancellation,omitzero"`
	inputFailure error
}

type StartCancellation struct {
	CommandID authoring.CommandID   `json:"commandId"`
	Replay    authoring.ReplayGuard `json:"replay"`
}

func (p PendingRun) Cancellation() *StartCancellation {
	if p.Dispatch == nil {
		return nil
	}
	return p.Dispatch.Cancellation
}

func (p PendingRun) dispatching() bool {
	return p.Dispatch != nil || p.claim == preparingPrompt
}

func (p PendingRun) entry() authoring.QueueEntry {
	return authoring.QueueEntry{
		ID: p.EntryID, CommandID: p.Command.CommandID, SessionID: p.Command.SessionID,
		Message: p.Command.Message.Clone(), Options: p.Command.Options.Clone(), Held: p.claim == editingPrompt,
	}
}

func (p PendingRun) clone() PendingRun {
	p.Command = p.Command.Clone()
	if p.Dispatch != nil {
		dispatch := *p.Dispatch
		if dispatch.Cancellation != nil {
			dispatch.Cancellation = new(*dispatch.Cancellation)
		}
		p.Dispatch = &dispatch
	}
	return p
}

func (p PendingRun) validate(sessionID string) error {
	if err := p.EntryID.Validate(); err != nil {
		return fmt.Errorf("queue entry: %w", err)
	}
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("command id is empty")
	}
	if p.Command.SessionID != sessionID {
		return fmt.Errorf("command belongs to session %s", p.Command.SessionID)
	}
	if p.Dispatch == nil {
		if p.Command.Input != nil {
			return errors.New("queued run has prepared input")
		}
		return nil
	}
	return p.Dispatch.validate()
}

func (d RunDispatch) validate() error {
	if err := d.InputDigest.validate(); err != nil {
		return err
	}
	if err := d.Replay.Validate(); err != nil {
		return err
	}
	if d.Cancellation == nil {
		return nil
	}
	if err := d.Cancellation.CommandID.Validate(); err != nil {
		return fmt.Errorf("cancel command: %w", err)
	}
	return d.Cancellation.Replay.Validate()
}

func (p PendingRun) prepare(replay authoring.ReplayGuard, blocks []protocol.ContentBlock, digest inputDigest) (PendingRun, error) {
	if p.claim != preparingPrompt || p.Dispatch != nil {
		return PendingRun{}, errors.New("prompt preparation no longer owns the queue head")
	}
	if err := replay.Validate(); err != nil {
		return PendingRun{}, err
	}
	p.claim = unclaimedPrompt
	p.Command.Input = blocks
	p.Dispatch = &RunDispatch{InputDigest: digest, Replay: replay}
	return p, nil
}

func (p PendingRun) beginCancellation(replay authoring.ReplayGuard) (PendingRun, error) {
	if p.Dispatch == nil {
		return PendingRun{}, errors.New("queued run has no dispatch to cancel")
	}
	if p.Dispatch.Cancellation != nil {
		return p, nil
	}
	if err := replay.Validate(); err != nil {
		return PendingRun{}, err
	}
	dispatch := *p.Dispatch
	dispatch.Cancellation = &StartCancellation{CommandID: mutation.NewCommandID(), Replay: replay}
	p.Dispatch = &dispatch
	return p, nil
}

func (p PendingRun) requeue() (PendingRun, error) {
	if p.Dispatch == nil || p.Dispatch.Cancellation != nil {
		return PendingRun{}, errors.New("only an active dispatch can be requeued")
	}
	if _, err := p.ReplayCommand(); err != nil {
		return PendingRun{}, err
	}
	p.Command.CommandID = mutation.NewCommandID()
	p.Command.Input = nil
	p.Dispatch = nil
	return p, nil
}

func validatePendingRunSequence(sessionID string, pending []PendingRun) error {
	entries := make(map[authoring.QueueEntryID]struct{}, len(pending))
	commands := make(map[authoring.CommandID]struct{}, len(pending))
	for index, entry := range pending {
		if err := entry.validate(sessionID); err != nil {
			return fmt.Errorf("pending run %d: %w", index+1, err)
		}
		if _, duplicate := entries[entry.EntryID]; duplicate {
			return fmt.Errorf("pending run %d repeats queue entry %s", index+1, entry.EntryID)
		}
		if _, duplicate := commands[entry.Command.CommandID]; duplicate {
			return fmt.Errorf("pending run %d repeats command %s", index+1, entry.Command.CommandID)
		}
		entries[entry.EntryID] = struct{}{}
		commands[entry.Command.CommandID] = struct{}{}
		if index > 0 && entry.dispatching() {
			return fmt.Errorf("pending run %d is dispatching behind the queue head", index+1)
		}
	}
	return nil
}

func pendingRunIndex(commands []PendingRun, commandID authoring.CommandID) int {
	return slices.IndexFunc(commands, func(command PendingRun) bool { return command.Command.CommandID == commandID })
}
