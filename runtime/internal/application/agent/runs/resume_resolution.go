package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// resolveResumeResponses validates exact item coverage and the kind-specific
// answer schema, then binds every decision to its exact executor input request.
// Output follows the pending barrier's canonical order, independent of request
// ordering, so every downstream layer observes one representation of the set.
func resolveResumeResponses(pending Pending, responses []ResumeResponse) ([]InterruptAnswer, error) {
	open := make(map[string]transcript.Interrupt, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
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
	resolutions := make(map[string]interrupt.Resolution, len(responses))
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
			itemResolution interrupt.Resolution
			err            error
		)
		switch request.Kind {
		case interrupt.Approval:
			itemResolution, err = resolveApprovalResponse(request, response)
		case interrupt.Question:
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
	if len(pending.Bindings) != len(pending.Interrupts) {
		return nil, fmt.Errorf(
			"%w: pending barrier has %d input-request bindings for %d items",
			ErrInvalidInterruptResponse,
			len(pending.Bindings),
			len(pending.Interrupts),
		)
	}
	answers := make([]InterruptAnswer, len(pending.Bindings))
	for index, binding := range pending.Bindings {
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

func resolveApprovalResponse(request transcript.Interrupt, response ResumeResponse) (interrupt.Resolution, error) {
	if response.Kind != ApprovalResponseKind || response.Approval == nil || response.Question != nil {
		return interrupt.Resolution{}, errors.New("approval response is required")
	}
	approval := response.Approval
	resolution := interrupt.Resolution{
		Approved:      approval.Approved,
		Arguments:     approval.Arguments,
		Reason:        strings.TrimSpace(approval.Reason),
		RememberScope: approval.RememberScope,
	}
	answer := InterruptAnswer{InterruptItemID: response.ItemID, Resolution: resolution}
	if err := answer.validateResolution(request); err != nil {
		return interrupt.Resolution{}, err
	}
	return resolution, nil
}

func resolveQuestionResponse(request transcript.Interrupt, response ResumeResponse) (interrupt.Resolution, error) {
	if response.Kind != QuestionResponseKind || response.Question == nil || response.Approval != nil {
		return interrupt.Resolution{}, errors.New("question response is required")
	}
	if request.Question == nil || len(request.Question.Fields) == 0 {
		return interrupt.Resolution{}, errors.New("open question has no fields")
	}
	resolution := interrupt.Resolution{Approved: true, Answers: cloneAnswers(response.Question.Answers)}
	answer := InterruptAnswer{InterruptItemID: response.ItemID, Resolution: resolution}
	if err := answer.validateResolution(request); err != nil {
		return interrupt.Resolution{}, err
	}
	return resolution, nil
}

// validateResolution checks the answer together with the exact interrupt that
// gives its fields meaning. Resolution is shared executor vocabulary, so its
// kind-specific invariant cannot live on that transport-shaped value alone.
func (a InterruptAnswer) validateResolution(request transcript.Interrupt) error {
	switch request.Kind {
	case interrupt.Approval:
		return a.validateApprovalResolution(request.Approval)
	case interrupt.Question:
		return a.validateQuestionResolution(request.Question)
	default:
		return fmt.Errorf("unknown interrupt kind %q", request.Kind)
	}
}

func (a InterruptAnswer) validateApprovalResolution(request *transcript.Approval) error {
	if request == nil {
		return errors.New("approval request has no prompt")
	}
	resolution := a.Resolution
	if resolution.Answers != nil {
		return errors.New("approval resolution cannot carry question answers")
	}
	if resolution.RememberScope != "" && !resolution.RememberScope.Valid() {
		return fmt.Errorf("unknown remember scope %q", resolution.RememberScope)
	}
	if resolution.RememberScope != "" && !request.Rememberable {
		return errors.New("approval cannot be remembered")
	}
	if resolution.Arguments != "" {
		if !resolution.Approved {
			return errors.New("denial cannot edit arguments")
		}
		if err := validateArguments(resolution.Arguments); err != nil {
			return fmt.Errorf("edited arguments: %w", err)
		}
	}
	if resolution.Reason != strings.TrimSpace(resolution.Reason) {
		return errors.New("denial reason has surrounding whitespace")
	}
	if resolution.Approved && resolution.Reason != "" {
		return errors.New("approval cannot carry a denial reason")
	}
	return nil
}

func (a InterruptAnswer) validateQuestionResolution(request *transcript.Question) error {
	if request == nil {
		return errors.New("question request has no prompt")
	}
	resolution := a.Resolution
	if !resolution.Approved {
		return errors.New("question resolution must acknowledge the answer")
	}
	if resolution.Arguments != "" || resolution.Reason != "" || resolution.RememberScope != "" {
		return errors.New("question resolution cannot carry approval fields")
	}
	if len(resolution.Answers) != len(request.Fields) {
		return &QuestionAnswerError{
			ItemID: a.InterruptItemID,
			Index:  -1,
			Detail: fmt.Sprintf(
				"must contain %d entries, got %d",
				len(request.Fields), len(resolution.Answers),
			),
		}
	}
	for index, field := range request.Fields {
		if err := field.ValidateAnswer(resolution.Answers[index]); err != nil {
			return &QuestionAnswerError{
				ItemID: a.InterruptItemID,
				Index:  index,
				Detail: err.Error(),
			}
		}
	}
	return nil
}

func cloneAnswers(answers [][]string) [][]string {
	if answers == nil {
		return nil
	}
	cloned := make([][]string, len(answers))
	for index, values := range answers {
		cloned[index] = slices.Clone(values)
	}
	return cloned
}
