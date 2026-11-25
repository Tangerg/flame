package workbench

import (
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type PendingResume struct {
	InputDigest  inputDigest `json:"inputDigest,omitzero"`
	inputFailure error
	Command      conversation.ResumeRun   `json:"-"`
	Interrupts   []conversation.Interrupt `json:"interrupts"`
	Replay       authoring.ReplayGuard    `json:"replay"`
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
	CommandID   authoring.CommandID    `json:"commandId"`
	RunID       string                 `json:"runId"`
	Message     *authoring.Message     `json:"message,omitzero"`
	Interrupts  []pendingInterruptJSON `json:"interrupts"`
	Replay      authoring.ReplayGuard  `json:"replay"`
}

type pendingInterruptType string

const (
	pendingApprovalInterrupt pendingInterruptType = "approval"
	pendingQuestionInterrupt pendingInterruptType = "question"
)

type pendingInterruptJSON struct {
	Type           pendingInterruptType         `json:"type"`
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
			Type: pendingApprovalInterrupt, Approval: &cloned, ApprovalAnswer: &decision,
		}, nil
	case conversation.Question:
		response, ok := conversation.CloneAnswer(answer).(conversation.QuestionAnswer)
		if !ok {
			return pendingInterruptJSON{}, errors.New("pending question has another answer kind")
		}
		cloned := item.Clone()
		return pendingInterruptJSON{
			Type: pendingQuestionInterrupt, Question: &cloned, QuestionAnswer: &response,
		}, nil
	default:
		return pendingInterruptJSON{}, fmt.Errorf("pending resume: %w: %T", conversation.ErrUnsupportedInterrupt, interrupt)
	}
}

func (p pendingInterruptJSON) decode(index int) (conversation.Interrupt, conversation.InterruptAnswer, error) {
	switch p.Type {
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
			p.Type,
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
