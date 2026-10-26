package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// resolveResumeResponses validates exact item coverage and the kind-specific
// answer schema, then binds every decision to its exact executor input request.
// interrupts are a hand-off's open interrupts projected from their Items, and
// bindings its input-request bindings in the same order. Output follows that
// canonical order, independent of request ordering, so every downstream layer
// observes one representation of the set.
func resolveResumeResponses(interrupts []transcript.Interrupt, bindings []InterruptBinding, responses []ResumeResponse) ([]InterruptAnswer, error) {
	open := make(map[string]transcript.Interrupt, len(interrupts))
	for _, interrupt := range interrupts {
		if interrupt.ItemID == "" {
			return nil, fmt.Errorf("%w: open interrupt has no item id", ErrInvalidInterruptResponse)
		}
		if _, exists := open[interrupt.ItemID]; exists {
			return nil, fmt.Errorf("%w: duplicate open item %q", ErrInvalidInterruptResponse, interrupt.ItemID)
		}
		open[interrupt.ItemID] = interrupt
	}
	if len(open) == 0 {
		return nil, ErrInterruptNotOpen
	}

	seen := make(map[string]struct{}, len(responses))
	resolutions := make(map[string]run.InterruptResolution, len(responses))
	for _, response := range responses {
		request, exists := open[response.ItemID]
		if !exists {
			return nil, fmt.Errorf("%w: item %q", ErrInterruptNotOpen, response.ItemID)
		}
		if _, duplicate := seen[response.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate response for item %q", ErrInvalidInterruptResponse, response.ItemID)
		}
		seen[response.ItemID] = struct{}{}

		var (
			itemResolution run.InterruptResolution
			err            error
		)
		switch request.Kind {
		case run.InterruptApproval:
			itemResolution, err = resolveApprovalResponse(request, response)
		case run.InterruptQuestion:
			itemResolution, err = resolveQuestionResponse(request, response)
		default:
			err = fmt.Errorf("unknown open interrupt kind %q", request.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: item %q: %w", ErrInvalidInterruptResponse, response.ItemID, err)
		}
		resolutions[response.ItemID] = itemResolution
	}
	if len(seen) != len(open) {
		return nil, fmt.Errorf(
			"%w: responses cover %d of %d open items",
			ErrInvalidInterruptResponse, len(seen), len(open),
		)
	}
	if len(bindings) != len(interrupts) {
		return nil, fmt.Errorf(
			"%w: pending barrier has %d input-request bindings for %d items",
			ErrInvalidInterruptResponse,
			len(bindings),
			len(interrupts),
		)
	}
	answers := make([]InterruptAnswer, len(bindings))
	for index, binding := range bindings {
		resolution, ok := resolutions[binding.InterruptItemID]
		if !ok {
			return nil, fmt.Errorf(
				"%w: input-request binding names unanswered item %q",
				ErrInvalidInterruptResponse,
				binding.InterruptItemID,
			)
		}
		answers[index] = InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      resolution,
		}
	}
	return answers, nil
}

func resolveApprovalResponse(request transcript.Interrupt, response ResumeResponse) (run.InterruptResolution, error) {
	if response.Kind != run.InterruptApproval || response.Approval == nil || response.Question != nil {
		return run.InterruptResolution{}, errors.New("approval response is required")
	}
	value := response.Approval
	resolution, err := run.NewApprovalAnswer(approval.Resolution{
		Decision: approval.DecisionOf(value.Approved), EditedArguments: value.Arguments,
		Reason: strings.TrimSpace(value.Reason), RememberScope: value.RememberScope,
	})
	if err != nil {
		return run.InterruptResolution{}, err
	}
	answer := InterruptAnswer{InterruptItemID: response.ItemID, Resolution: resolution}
	if err := answer.validateResolution(request); err != nil {
		return run.InterruptResolution{}, err
	}
	return resolution, nil
}

func resolveQuestionResponse(request transcript.Interrupt, response ResumeResponse) (run.InterruptResolution, error) {
	if response.Kind != run.InterruptQuestion || response.Question == nil || response.Approval != nil {
		return run.InterruptResolution{}, errors.New("question response is required")
	}
	if request.Question == nil || len(request.Question.Fields) == 0 {
		return run.InterruptResolution{}, errors.New("open question has no fields")
	}
	resolution, err := run.NewQuestionAnswer(response.Question.Answers)
	if err != nil {
		return run.InterruptResolution{}, err
	}
	answer := InterruptAnswer{InterruptItemID: response.ItemID, Resolution: resolution}
	if err := answer.validateResolution(request); err != nil {
		return run.InterruptResolution{}, err
	}
	return resolution, nil
}

// validateResolution checks the closed answer against its exact open request.
// Rememberability and question fields belong to that request.
func (a InterruptAnswer) validateResolution(request transcript.Interrupt) error {
	if a.Resolution.Kind() != request.Kind {
		return errors.New("answer kind differs from its interrupt")
	}
	switch request.Kind {
	case run.InterruptApproval:
		return a.validateApprovalResolution(request.Approval)
	case run.InterruptQuestion:
		return a.validateQuestionResolution(request.Question)
	default:
		return fmt.Errorf("unknown interrupt kind %q", request.Kind)
	}
}

func (a InterruptAnswer) validateApprovalResolution(request *transcript.Approval) error {
	if request == nil {
		return errors.New("approval request has no prompt")
	}
	resolution := a.Resolution.Approval()
	if resolution.RememberScope != "" && !request.Rememberable {
		return errors.New("approval cannot be remembered")
	}
	return nil
}

func (a InterruptAnswer) validateQuestionResolution(request *transcript.Question) error {
	if request == nil {
		return errors.New("question request has no prompt")
	}
	answers := a.Resolution.Question().Answers()
	if len(answers) != len(request.Fields) {
		return &QuestionAnswerError{
			ItemID: a.InterruptItemID,
			Index:  -1,
			Detail: fmt.Sprintf(
				"must contain %d entries, got %d",
				len(request.Fields), len(answers),
			),
		}
	}
	for index, field := range request.Fields {
		if err := field.ValidateAnswer(answers[index]); err != nil {
			return &QuestionAnswerError{
				ItemID: a.InterruptItemID,
				Index:  index,
				Detail: err.Error(),
			}
		}
	}
	return nil
}
