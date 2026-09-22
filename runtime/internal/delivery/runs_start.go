package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	corechat "github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/protocol"
)

// StartRun translates runs.start into the in-process execution path. It returns
// the runId synchronously; events flow
// out via the returned sequence as RunEvents (wrapped by the transport
// into notifications.run.event). The terminal segment.finished rides this
// sequence — including outcome:interrupt when the run parks for HITL
// approval, after which the run suspends and the client answers via
// runs.resume.
func (s *Handler) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	options := generationOptionsFromWire(in.Params)
	selection, err := modelref.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	// Negotiated before admission: the Run is created under this contract and keeps
	// it for life, so a capability we cannot honor has to stop the call rather than
	// be discovered halfway through its stream.
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.runs.Start(ctx, runs.StartCommand{
		SessionID:      in.SessionID,
		ModelSelection: selection,
		Options:        options,
		Capabilities:   capabilities,
		Input:          input,
	})
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: result.UserItemID}, mapRunEvents(result.Events), nil
}

func decodeRunInput(blocks []protocol.ContentBlock) ([]transcript.ContentBlock, error) {
	input := make([]transcript.ContentBlock, len(blocks))
	for i, block := range blocks {
		decoded, decodeErr := decodeContent(encodedContent{
			kind: block.Type, text: block.Text, mime: block.Mime, data: block.Data,
		})
		if decodeErr != nil {
			return nil, invalidWireContentBlock(i, decodeErr.field, decodeErr.detail)
		}
		input[i] = decoded
	}
	return input, nil
}

func invalidWireContentBlock(index int, field, detail string) error {
	constraint := &protocol.ConstraintError{Shape: "RunInput", Fields: []protocol.FieldError{{
		Field: fmt.Sprintf("input[%d].%s", index, field), Detail: detail,
	}}}
	return NewFailure(errors.Join(protocol.ErrInvalidParams, constraint), constraint.Error())
}

func wireRunStartErr(err error) error {
	// A session that already has a run is refused WITH that run: the client offers
	// steer / resume / cancel, and the runtime cancels nothing on its own.
	if conflict, ok := errors.AsType[*runs.ActiveRunConflictError](err); ok {
		return &ActiveRunConflictError{ActiveRun: protocol.ActiveRunRef{
			RunID: conflict.RunID, Status: presentRunStatus(conflict.Status),
		}}
	}
	switch {
	case errors.Is(err, runs.ErrInputRequired):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), "input must contain a user text or image block")
	case modelref.IsInvalid(err):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrInvalidRunOptions):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrUnsupportedModelSelection):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrInvalidScheduledStart):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	default:
		return err
	}
}

func generationOptionsFromWire(in *protocol.GenerationParams) *corechat.Options {
	if in == nil {
		return nil
	}
	return &corechat.Options{
		Temperature:     in.Temperature,
		MaxOutputTokens: in.MaxTokens,
		TopP:            in.TopP,
		Stop:            slices.Clone(in.Stop),
	}
}
