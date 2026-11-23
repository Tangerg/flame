package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func settleResumeAnswers(
	interrupts []transcript.Interrupt,
	items map[identity.ItemID]transcript.Item,
	responses []ResumeResponse,
) ([]transcript.Replacement, error) {
	open := make(map[identity.ItemID]transcript.Interrupt, len(interrupts))
	for _, interrupt := range interrupts {
		open[interrupt.ItemID] = interrupt
	}
	settled := make(map[identity.ItemID]transcript.Replacement, len(responses))
	for _, response := range responses {
		request, found := open[response.ItemID]
		if !found {
			return nil, fmt.Errorf("%w: item %q", ErrInterruptNotOpen, response.ItemID)
		}
		if _, duplicate := settled[response.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate response for item %q", ErrInvalidInterruptResponse, response.ItemID)
		}
		change, err := settleResumeAnswer(items[request.ItemID], request.Prompt, response)
		if err != nil {
			return nil, fmt.Errorf("%w: item %q: %w", ErrInvalidInterruptResponse, response.ItemID, err)
		}
		settled[response.ItemID] = change
	}
	if len(settled) != len(interrupts) {
		return nil, fmt.Errorf("%w: responses cover %d of %d open items", ErrInvalidInterruptResponse, len(settled), len(interrupts))
	}
	changes := make([]transcript.Replacement, len(interrupts))
	for index, interrupt := range interrupts {
		changes[index] = settled[interrupt.ItemID]
	}
	return changes, nil
}

func settleResumeAnswer(item transcript.Item, prompt transcript.Prompt, response ResumeResponse) (transcript.Replacement, error) {
	switch prompt := prompt.(type) {
	case transcript.Approval:
		if response.Kind != run.InterruptApproval || response.Approval == nil || response.Question != nil {
			return transcript.Replacement{}, errors.New("approval response is required")
		}
		answer := response.Approval
		if answer.RememberScope != "" && !prompt.Rememberable {
			return transcript.Replacement{}, errors.New("approval cannot be remembered")
		}
		return item.ResolveToolApproval(approval.Resolution{
			Decision: approval.DecisionOf(answer.Approved), EditedArguments: answer.Arguments,
			Reason: strings.TrimSpace(answer.Reason), RememberScope: answer.RememberScope,
		})
	case transcript.Question:
		if response.Kind != run.InterruptQuestion || response.Question == nil || response.Approval != nil {
			return transcript.Replacement{}, errors.New("question response is required")
		}
		change, err := item.AnswerQuestion(response.Question.Answers)
		if err == nil {
			return change, nil
		}
		index, detail := -1, err.Error()
		if field, ok := errors.AsType[*questioncontract.FieldError](err); ok {
			index, detail = field.Index, field.Cause.Error()
		}
		return transcript.Replacement{}, &QuestionAnswerError{ItemID: item.ID(), Index: index, Detail: detail}
	default:
		return transcript.Replacement{}, fmt.Errorf("unknown interrupt prompt %T", prompt)
	}
}

func (p Pending) settledAnswers(sessionID identity.SessionID, items map[identity.ItemID]transcript.Item) ([]InterruptAnswer, error) {
	answers := make([]InterruptAnswer, 0, len(p.Bindings))
	for index, binding := range p.Bindings {
		item, found := items[binding.InterruptItemID]
		if !found {
			return nil, fmt.Errorf("runs: accepted continuation Item %q is missing", binding.InterruptItemID)
		}
		member, _ := continuationForMember(p.Continuations, binding.MemberID)
		if item.ID() != binding.InterruptItemID || item.SessionID() != sessionID || item.RunID() != member.RunID {
			return nil, fmt.Errorf("runs: accepted continuation Item %q belongs to another owner", binding.InterruptItemID)
		}
		resolution, err := settledItemResolution(item)
		if err != nil {
			return nil, fmt.Errorf("runs: accepted continuation Item %q: %w", binding.InterruptItemID, err)
		}
		if resolution.Type() != p.Interrupts[index].Type() {
			return nil, fmt.Errorf("runs: accepted continuation Item %q has a resolution of another kind", binding.InterruptItemID)
		}
		answers = append(answers, InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      resolution,
		})
	}
	return answers, nil
}

func settledItemResolution(item transcript.Item) (run.InterruptResolution, error) {
	if question, ok := item.Question(); ok {
		if !question.Answered() {
			return run.InterruptResolution{}, errors.New("question Item is not answered")
		}
		return run.NewQuestionAnswer(question.Answers)
	}
	call, ok := item.ToolCall()
	if !ok || call.Approval == nil {
		return run.InterruptResolution{}, errors.New("approval Item has no accepted resolution")
	}
	return run.NewApprovalAnswer(*call.Approval)
}
