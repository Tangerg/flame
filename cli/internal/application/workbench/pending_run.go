package workbench

import (
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

// PendingRunState is the durable delivery phase of one start command.
type PendingRunState string

const (
	PendingRunQueued      PendingRunState = "queued"
	PendingRunDispatching PendingRunState = "dispatching"
	PendingRunCanceling   PendingRunState = "canceling"
)

// PendingRun is one durable runtime outbox entry. State distinguishes intent
// that has never left the queue from an ambiguous command handshake.
type PendingRun struct {
	InputDigest     inputDigest `json:"inputDigest,omitzero"`
	inputFailure    error
	State           PendingRunState  `json:"state"`
	Command         prompt.StartRun  `json:"command"`
	Replay          replay.Guard     `json:"replay"`
	CancelCommandID replay.CommandID `json:"cancelCommandId,omitzero"`
	CancelReplay    replay.Guard     `json:"cancelReplay"`
}

// PendingResume is a HITL decision whose command may already have reached the
// runtime. It remains durable until the runtime either acknowledges the exact
// command identity or definitively rejects it. Command addresses the root;
// Interrupts retain the members from the authoritative waiting set reviewed
// by the terminal before staging.
type PendingResume struct {
	InputDigest  inputDigest `json:"inputDigest,omitzero"`
	inputFailure error
	Command      conversation.ResumeRun   `json:"-"`
	Interrupts   []conversation.Interrupt `json:"interrupts"`
	Replay       replay.Guard             `json:"replay"`
}

func (p PendingResume) validate() error {
	if err := p.InputDigest.validate(); err != nil {
		return err
	}
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("resume command id is empty")
	}
	if err := conversation.ValidateInterrupts(p.Interrupts); err != nil {
		return err
	}
	if len(p.Command.Answers) != len(p.Interrupts) {
		return errors.New("resume answer count does not match interrupts")
	}
	for index, interrupt := range p.Interrupts {
		response := p.Command.Answers[index]
		if response.ItemID != conversation.InterruptItemID(interrupt) {
			return fmt.Errorf("resume answer %d targets another interrupt", index+1)
		}
		if err := conversation.ValidateAnswer(interrupt, response.Answer); err != nil {
			return fmt.Errorf("resume answer %d: %w", index+1, err)
		}
	}
	return nil
}

type pendingResumeJSON struct {
	InputDigest inputDigest            `json:"inputDigest,omitzero"`
	CommandID   replay.CommandID       `json:"commandId"`
	RunID       string                 `json:"runId"`
	Message     *prompt.Message        `json:"message,omitzero"`
	Interrupts  []pendingInterruptJSON `json:"interrupts"`
	Replay      replay.Guard           `json:"replay"`
}

type pendingInterruptKind string

const (
	pendingApprovalInterrupt pendingInterruptKind = "approval"
	pendingQuestionInterrupt pendingInterruptKind = "question"
)

type pendingInterruptJSON struct {
	Kind           pendingInterruptKind         `json:"kind"`
	Approval       *conversation.Approval       `json:"approval,omitzero"`
	Question       *conversation.Question       `json:"question,omitzero"`
	ApprovalAnswer *conversation.ApprovalAnswer `json:"approvalAnswer,omitzero"`
	QuestionAnswer *conversation.QuestionAnswer `json:"questionAnswer,omitzero"`
}

func newPendingInterruptJSON(
	interrupt conversation.Interrupt,
	answer conversation.Answer,
) (pendingInterruptJSON, error) {
	switch item := interrupt.(type) {
	case conversation.Approval:
		decision, ok := answer.(conversation.ApprovalAnswer)
		if !ok {
			return pendingInterruptJSON{}, errors.New("pending approval has another answer kind")
		}
		cloned := item.Clone()
		return pendingInterruptJSON{
			Kind: pendingApprovalInterrupt, Approval: &cloned, ApprovalAnswer: &decision,
		}, nil
	case conversation.Question:
		response, ok := conversation.CloneAnswer(answer).(conversation.QuestionAnswer)
		if !ok {
			return pendingInterruptJSON{}, errors.New("pending question has another answer kind")
		}
		cloned := item.Clone()
		return pendingInterruptJSON{
			Kind: pendingQuestionInterrupt, Question: &cloned, QuestionAnswer: &response,
		}, nil
	default:
		return pendingInterruptJSON{}, fmt.Errorf("pending resume has unknown interrupt %T", interrupt)
	}
}

func (p pendingInterruptJSON) decode(index int) (conversation.Interrupt, conversation.InterruptAnswer, error) {
	switch p.Kind {
	case pendingApprovalInterrupt:
		if p.Approval == nil || p.ApprovalAnswer == nil || p.Question != nil || p.QuestionAnswer != nil {
			return nil, conversation.InterruptAnswer{}, fmt.Errorf(
				"pending resume interrupt %d has an invalid approval shape",
				index+1,
			)
		}
		return p.Approval.Clone(), conversation.InterruptAnswer{
			ItemID: p.Approval.ItemID,
			Answer: *p.ApprovalAnswer,
		}, nil
	case pendingQuestionInterrupt:
		if p.Question == nil || p.QuestionAnswer == nil || p.Approval != nil || p.ApprovalAnswer != nil {
			return nil, conversation.InterruptAnswer{}, fmt.Errorf(
				"pending resume interrupt %d has an invalid question shape",
				index+1,
			)
		}
		return p.Question.Clone(), conversation.InterruptAnswer{
			ItemID: p.Question.ItemID,
			Answer: conversation.CloneAnswer(*p.QuestionAnswer),
		}, nil
	default:
		return nil, conversation.InterruptAnswer{}, fmt.Errorf(
			"pending resume interrupt %d has unknown kind %q",
			index+1,
			p.Kind,
		)
	}
}

func (p PendingResume) MarshalJSON() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	wire := pendingResumeJSON{
		InputDigest: p.InputDigest,
		CommandID:   p.Command.CommandID,
		RunID:       p.Command.RunID,
		Message:     p.Command.Message,
		Interrupts:  make([]pendingInterruptJSON, len(p.Interrupts)),
		Replay:      p.Replay,
	}
	for index, interrupt := range p.Interrupts {
		encoded, err := newPendingInterruptJSON(interrupt, p.Command.Answers[index].Answer)
		if err != nil {
			return nil, err
		}
		wire.Interrupts[index] = encoded
	}
	// The record travels inside a state file, so it is encoded under the same
	// options: the store owns the durable representations, not this type.
	return json.Marshal(wire, stateJSONOptions)
}

func (p *PendingResume) UnmarshalJSON(encoded []byte) error {
	var wire pendingResumeJSON
	if err := decodeStateJSON(encoded, &wire); err != nil {
		return err
	}
	decoded := PendingResume{
		InputDigest: wire.InputDigest,
		Command: conversation.ResumeRun{
			CommandID: wire.CommandID, RunID: wire.RunID, Message: wire.Message,
			Answers: make([]conversation.InterruptAnswer, len(wire.Interrupts)),
		},
		Interrupts: make([]conversation.Interrupt, len(wire.Interrupts)),
		Replay:     wire.Replay,
	}
	for index, item := range wire.Interrupts {
		interrupt, answer, err := item.decode(index)
		if err != nil {
			return err
		}
		decoded.Interrupts[index] = interrupt
		decoded.Command.Answers[index] = answer
	}
	if err := decoded.validate(); err != nil {
		return err
	}
	*p = clonePendingResume(decoded)
	return nil
}

func (p PendingRun) validate(sessionID string) error {
	if err := p.InputDigest.validate(); err != nil {
		return err
	}
	if p.State != PendingRunQueued && p.State != PendingRunDispatching && p.State != PendingRunCanceling {
		return fmt.Errorf("state %q is invalid", p.State)
	}
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("command id is empty")
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if err := p.CancelReplay.Validate(); err != nil {
		return err
	}
	switch p.State {
	case PendingRunCanceling:
		if err := p.CancelCommandID.Validate(); err != nil {
			return fmt.Errorf("cancel command: %w", err)
		}
	default:
		if p.CancelCommandID != "" {
			return errors.New("non-canceling run carries a cancel command")
		}
	}
	if p.State == PendingRunQueued && (p.Replay.Protected() || p.CancelReplay.Protected()) {
		return errors.New("queued run carries a runtime replay guard")
	}
	if p.State == PendingRunQueued && (p.InputDigest != "" || p.Command.Input != nil) {
		return errors.New("queued run carries prepared input")
	}
	if p.Command.SessionID != sessionID {
		return fmt.Errorf("command belongs to session %s", p.Command.SessionID)
	}
	return nil
}

func (p *PendingRun) beginDispatch(replayGuard replay.Guard) error {
	if err := replayGuard.Validate(); err != nil {
		return err
	}
	switch p.State {
	case PendingRunQueued:
		p.State = PendingRunDispatching
		p.Replay = replayGuard
		return nil
	case PendingRunDispatching, PendingRunCanceling:
		return nil
	default:
		return fmt.Errorf("pending run cannot begin dispatch from %q", p.State)
	}
}

func (p *PendingRun) beginCancellation(
	replayGuard replay.Guard,
) (replay.CommandID, error) {
	if err := replayGuard.Validate(); err != nil {
		return "", err
	}
	switch p.State {
	case PendingRunDispatching:
		cancelCommandID := mutation.NewCommandID()
		p.State = PendingRunCanceling
		p.CancelCommandID = cancelCommandID
		p.CancelReplay = replayGuard
		return cancelCommandID, nil
	case PendingRunCanceling:
		return p.CancelCommandID, nil
	default:
		return "", fmt.Errorf("pending run cannot begin cancellation from %q", p.State)
	}
}

func (p *PendingRun) requeue() (replay.CommandID, error) {
	if p.State != PendingRunDispatching {
		return "", fmt.Errorf("pending run cannot be requeued from %q", p.State)
	}
	if _, err := p.ReplayCommand(); err != nil {
		return "", err
	}
	replacement := mutation.NewCommandID()
	p.State = PendingRunQueued
	p.Command.CommandID = replacement
	p.Command.Input = nil
	p.InputDigest = ""
	p.CancelCommandID = ""
	p.Replay = replay.UnprotectedGuard()
	p.CancelReplay = replay.UnprotectedGuard()
	return replacement, nil
}

func (p PendingRun) acknowledgeable() error {
	if p.State != PendingRunDispatching && p.State != PendingRunCanceling {
		return fmt.Errorf("pending run cannot be acknowledged from %q", p.State)
	}
	return nil
}

func validatePendingRunSequence(sessionID string, pending []PendingRun) error {
	seen := make(map[replay.CommandID]struct{}, len(pending))
	for index, command := range pending {
		if err := command.validate(sessionID); err != nil {
			return fmt.Errorf("pending run %d: %w", index+1, err)
		}
		if _, duplicate := seen[command.Command.CommandID]; duplicate {
			return fmt.Errorf("pending run %d repeats command %s", index+1, command.Command.CommandID)
		}
		seen[command.Command.CommandID] = struct{}{}
		if index > 0 && command.State != PendingRunQueued {
			return fmt.Errorf("pending run %d is %s behind the FIFO boundary", index+1, command.State)
		}
	}
	return nil
}
