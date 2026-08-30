package workbench

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/commandreplay"
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
	State           PendingRunState     `json:"state"`
	Command         agent.StartRun      `json:"command"`
	Replay          commandreplay.Guard `json:"replay"`
	CancelCommandID agent.CommandID     `json:"cancelCommandId,omitempty"`
	CancelReplay    commandreplay.Guard `json:"cancelReplay"`
}

// PendingResume is a HITL decision whose command may already have reached the
// runtime. It remains durable until the runtime either acknowledges the exact
// command identity or definitively rejects it.
type PendingResume struct {
	Command      agent.ResumeRun     `json:"-"`
	Interactions []agent.Interaction `json:"interactions"`
	Replay       commandreplay.Guard `json:"replay"`
}

func (p PendingResume) validate() error {
	if err := p.Command.Validate(); err != nil {
		return err
	}
	if err := p.Replay.Validate(); err != nil {
		return err
	}
	if p.Command.CommandID == "" {
		return errors.New("resume command id is empty")
	}
	if err := agent.ValidateInteractions(p.Interactions); err != nil {
		return err
	}
	for index, interaction := range p.Interactions {
		if agent.InteractionRunID(interaction) != p.Command.RunID {
			return fmt.Errorf("interaction %d belongs to another run", index+1)
		}
	}
	if len(p.Command.Answers) != len(p.Interactions) {
		return errors.New("resume answer count does not match interactions")
	}
	for index, interaction := range p.Interactions {
		response := p.Command.Answers[index]
		if response.ItemID != agent.InteractionItemID(interaction) {
			return fmt.Errorf("resume answer %d targets another interaction", index+1)
		}
		if err := agent.ValidateAnswer(interaction, response.Answer); err != nil {
			return fmt.Errorf("resume answer %d: %w", index+1, err)
		}
	}
	return nil
}

type pendingResumeJSON struct {
	CommandID    agent.CommandID          `json:"commandId"`
	RunID        string                   `json:"runId"`
	Message      *agent.Message           `json:"message,omitempty"`
	Interactions []pendingInteractionJSON `json:"interactions"`
	Replay       commandreplay.Guard      `json:"replay"`
}

type pendingInteractionKind string

const (
	pendingApprovalInteraction pendingInteractionKind = "approval"
	pendingQuestionInteraction pendingInteractionKind = "question"
)

type pendingInteractionJSON struct {
	Kind           pendingInteractionKind `json:"kind"`
	Approval       *agent.Approval        `json:"approval,omitempty"`
	Question       *agent.Question        `json:"question,omitempty"`
	ApprovalAnswer *agent.ApprovalAnswer  `json:"approvalAnswer,omitempty"`
	QuestionAnswer *agent.QuestionAnswer  `json:"questionAnswer,omitempty"`
}

func newPendingInteractionJSON(
	interaction agent.Interaction,
	answer agent.Answer,
) (pendingInteractionJSON, error) {
	switch item := interaction.(type) {
	case agent.Approval:
		decision, ok := answer.(agent.ApprovalAnswer)
		if !ok {
			return pendingInteractionJSON{}, errors.New("pending approval has another answer kind")
		}
		cloned := item.Clone()
		return pendingInteractionJSON{
			Kind: pendingApprovalInteraction, Approval: &cloned, ApprovalAnswer: &decision,
		}, nil
	case agent.Question:
		response, ok := agent.CloneAnswer(answer).(agent.QuestionAnswer)
		if !ok {
			return pendingInteractionJSON{}, errors.New("pending question has another answer kind")
		}
		cloned := item.Clone()
		return pendingInteractionJSON{
			Kind: pendingQuestionInteraction, Question: &cloned, QuestionAnswer: &response,
		}, nil
	default:
		return pendingInteractionJSON{}, fmt.Errorf("pending resume has unknown interaction %T", interaction)
	}
}

func (p pendingInteractionJSON) decode(index int) (agent.Interaction, agent.InterruptAnswer, error) {
	switch p.Kind {
	case pendingApprovalInteraction:
		if p.Approval == nil || p.ApprovalAnswer == nil || p.Question != nil || p.QuestionAnswer != nil {
			return nil, agent.InterruptAnswer{}, fmt.Errorf(
				"pending resume interaction %d has an invalid approval shape",
				index+1,
			)
		}
		return p.Approval.Clone(), agent.InterruptAnswer{
			ItemID: p.Approval.ItemID,
			Answer: *p.ApprovalAnswer,
		}, nil
	case pendingQuestionInteraction:
		if p.Question == nil || p.QuestionAnswer == nil || p.Approval != nil || p.ApprovalAnswer != nil {
			return nil, agent.InterruptAnswer{}, fmt.Errorf(
				"pending resume interaction %d has an invalid question shape",
				index+1,
			)
		}
		return p.Question.Clone(), agent.InterruptAnswer{
			ItemID: p.Question.ItemID,
			Answer: agent.CloneAnswer(*p.QuestionAnswer),
		}, nil
	default:
		return nil, agent.InterruptAnswer{}, fmt.Errorf(
			"pending resume interaction %d has unknown kind %q",
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
		CommandID:    p.Command.CommandID,
		RunID:        p.Command.RunID,
		Message:      p.Command.Message,
		Interactions: make([]pendingInteractionJSON, len(p.Interactions)),
		Replay:       p.Replay,
	}
	for index, interaction := range p.Interactions {
		encoded, err := newPendingInteractionJSON(interaction, p.Command.Answers[index].Answer)
		if err != nil {
			return nil, err
		}
		wire.Interactions[index] = encoded
	}
	return json.Marshal(wire)
}

func (p *PendingResume) UnmarshalJSON(encoded []byte) error {
	var wire pendingResumeJSON
	if err := decodeStateJSON(encoded, &wire); err != nil {
		return err
	}
	decoded := PendingResume{
		Command: agent.ResumeRun{
			CommandID: wire.CommandID, RunID: wire.RunID, Message: wire.Message,
			Answers: make([]agent.InterruptAnswer, len(wire.Interactions)),
		},
		Interactions: make([]agent.Interaction, len(wire.Interactions)),
		Replay:       wire.Replay,
	}
	for index, item := range wire.Interactions {
		interaction, answer, err := item.decode(index)
		if err != nil {
			return err
		}
		decoded.Interactions[index] = interaction
		decoded.Command.Answers[index] = answer
	}
	if err := decoded.validate(); err != nil {
		return err
	}
	*p = clonePendingResume(decoded)
	return nil
}

func (p PendingRun) validate(sessionID string) error {
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
	if p.Command.SessionID != sessionID {
		return fmt.Errorf("command belongs to session %s", p.Command.SessionID)
	}
	return nil
}

func (p *PendingRun) beginDispatch(replay commandreplay.Guard) error {
	if err := replay.Validate(); err != nil {
		return err
	}
	switch p.State {
	case PendingRunQueued:
		p.State = PendingRunDispatching
		p.Replay = replay
		return nil
	case PendingRunDispatching, PendingRunCanceling:
		return nil
	default:
		return fmt.Errorf("pending run cannot begin dispatch from %q", p.State)
	}
}

func (p *PendingRun) beginCancellation(
	replay commandreplay.Guard,
	newCommandID func() (agent.CommandID, error),
) (agent.CommandID, error) {
	if err := replay.Validate(); err != nil {
		return "", err
	}
	switch p.State {
	case PendingRunDispatching:
		cancelCommandID, err := newCommandID()
		if err != nil {
			return "", err
		}
		p.State = PendingRunCanceling
		p.CancelCommandID = cancelCommandID
		p.CancelReplay = replay
		return cancelCommandID, nil
	case PendingRunCanceling:
		return p.CancelCommandID, nil
	default:
		return "", fmt.Errorf("pending run cannot begin cancellation from %q", p.State)
	}
}

func (p *PendingRun) requeue(newCommandID func() (agent.CommandID, error)) (agent.CommandID, error) {
	if p.State != PendingRunDispatching {
		return "", fmt.Errorf("pending run cannot be requeued from %q", p.State)
	}
	replacement, err := newCommandID()
	if err != nil {
		return "", err
	}
	p.State = PendingRunQueued
	p.Command.CommandID = replacement
	p.CancelCommandID = ""
	p.Replay = commandreplay.UnprotectedGuard()
	p.CancelReplay = commandreplay.UnprotectedGuard()
	return replacement, nil
}

func (p PendingRun) acknowledgeable() error {
	if p.State != PendingRunDispatching && p.State != PendingRunCanceling {
		return fmt.Errorf("pending run cannot be acknowledged from %q", p.State)
	}
	return nil
}

func validatePendingRunSequence(sessionID string, pending []PendingRun) error {
	seen := make(map[agent.CommandID]struct{}, len(pending))
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
