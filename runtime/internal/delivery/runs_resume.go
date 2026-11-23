package delivery

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framehost "github.com/Tangerg/frame/host"
)

func (s *Handler) ResumeRun(ctx context.Context, in protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	// Validate the decision BEFORE touching the interrupt — a malformed
	// response shouldn't consume the (still-resumable) record.
	responses, err := decodeResumeResponses(in.Responses)
	if err != nil {
		return nil, nil, err
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	runID, err := runParameter(in.RunID)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.runs.Resume(ctx, runs.ResumeCommand{
		RunID:              runID,
		Responses:          responses,
		Acceptance:         runCommandIdentity(ctx),
		Input:              input,
		CallerCapabilities: caller,
	})
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
		return nil, nil, capabilityGap(uncovered.Missing)
	}
	if answerError, ok := errors.AsType[*runs.QuestionAnswerError](err); ok {
		return nil, nil, questionAnswerParamsError(in.Responses, answerError)
	}
	if err != nil {
		return nil, nil, wireRunResumeErr(err)
	}
	response := &protocol.ResumeRunResponse{
		RunID: result.RunID.String(), SegmentID: result.SegmentID.String(),
	}
	if !result.UserItemID.IsZero() {
		userItemID := result.UserItemID.String()
		response.UserItemID = &userItemID
	}
	if result.Reopened() {
		events, err := s.acceptedRunEvents(ctx, result)
		return response, events, err
	}
	return response, mapRunEvents(result.Events), nil
}

func wireRunResumeErr(err error) error {
	switch {
	case errors.Is(err, runs.ErrCommandKeyConflict):
		return protocol.NewFailure(protocol.ErrIdempotencyConflict, errIdempotencyRebound.Error(), err)
	case errors.Is(err, runs.ErrInterruptNotOpen):
		return protocol.ErrInterruptNotOpen
	case errors.Is(err, runs.ErrInvalidInterruptResponse):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, run.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, runs.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, runs.ErrInstallationChanged), errors.Is(err, framehost.ErrStaleBinding):
		return pluginChanged(err)
	default:
		return err
	}
}

func decodeResumeResponses(responses []protocol.InterruptResponse) ([]runs.ResumeResponse, error) {
	out := make([]runs.ResumeResponse, 0, len(responses))
	for _, wire := range responses {
		response, err := decodeResumeResponse(wire)
		if err != nil {
			return nil, err
		}
		out = append(out, response)
	}
	return out, nil
}

func decodeResumeResponse(wire protocol.InterruptResponse) (runs.ResumeResponse, error) {
	itemID, err := wireIdentity("interrupt response itemId", wire.ItemID, identity.ParseItem)
	if err != nil {
		return runs.ResumeResponse{}, err
	}
	response := runs.ResumeResponse{ItemID: itemID}
	switch wire.Response.Type {
	case protocol.InterruptResponseApproval:
		approval, err := decodeApprovalResponse(wire.Response)
		if err != nil {
			return runs.ResumeResponse{}, err
		}
		response.Kind = run.InterruptApproval
		response.Approval = approval
	case protocol.InterruptResponseAnswer:
		question, err := decodeQuestionResponse(wire.Response)
		if err != nil {
			return runs.ResumeResponse{}, err
		}
		response.Kind = run.InterruptQuestion
		response.Question = question
	default:
		return runs.ResumeResponse{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown interrupt response type %q", wire.Response.Type))
	}
	return response, nil
}

func decodeApprovalResponse(wire protocol.InterruptResponseValue) (*runs.ApprovalResponse, error) {
	if wire.Answers != nil {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, "approval response contains fields for another response type")
	}
	approval := &runs.ApprovalResponse{Reason: wire.Reason}
	if wire.Remember != nil {
		scope, ok := rememberScopeFromWire(wire.Remember.Scope)
		if !ok {
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("remember scope must be %q | %q | %q", protocol.RememberSession, protocol.RememberProject, protocol.RememberGlobal))
		}
		approval.RememberScope = scope
	}
	switch wire.Decision {
	case protocol.ApprovalApprove:
		approval.Approved = true
		if wire.EditedArgs != nil {
			encoded, err := json.Marshal(wire.EditedArgs, json.Deterministic(true))
			if err != nil {
				return nil, fmt.Errorf("runs.resume: editedArgs: %w", err)
			}
			approval.Arguments = string(encoded)
		}
	case protocol.ApprovalDeny:
		approval.Approved = false
	default:
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, "approval decision must be \"approve\" | \"deny\"")
	}
	return approval, nil
}

func decodeQuestionResponse(wire protocol.InterruptResponseValue) (*runs.QuestionResponse, error) {
	if wire.Decision != "" || wire.Remember != nil || wire.EditedArgs != nil || wire.Reason != "" {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, "answer response contains fields for another response type")
	}
	return &runs.QuestionResponse{Answers: transcript.CloneAnswers(wire.Answers)}, nil
}

func questionAnswerParamsError(
	responses []protocol.InterruptResponse,
	answerError *runs.QuestionAnswerError,
) error {
	responseIndex := -1
	for index, response := range responses {
		if response.ItemID == answerError.ItemID.String() {
			responseIndex = index
			break
		}
	}
	field := "responses"
	if responseIndex >= 0 {
		field = fmt.Sprintf("responses[%d].response.answers", responseIndex)
		if answerError.Index >= 0 {
			field += fmt.Sprintf("[%d]", answerError.Index)
		}
	}
	constraint := &protocol.ConstraintError{
		Shape:  "ResumeRunRequest",
		Fields: []protocol.FieldError{{Field: field, Detail: answerError.Detail}},
	}
	return protocol.NewFailure(protocol.ErrInvalidParams, constraint.Error(), constraint)
}
