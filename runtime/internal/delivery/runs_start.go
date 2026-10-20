package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/flame/runtime/protocol"
	corechat "github.com/Tangerg/scope/core/chat"
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
	selection, err := modelchoice.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
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
	command := runs.StartCommand{
		SessionID:      in.SessionID,
		ModelSelection: selection,
		Options:        options,
		Capabilities:   capabilities,
		Input:          input,
		Acceptance:     runCommandIdentity(ctx),
	}
	if in.NewSession != nil {
		command.NewSession = &runs.SessionOpening{SessionID: in.NewSession.SessionID, RunID: in.NewSession.RunID, Title: in.NewSession.Title, CWD: in.NewSession.Workspace.Path}
	}
	if actor, present := operation.ActorFrom(ctx); present {
		command.Actor = &actor
	}
	result, err := s.runs.Start(ctx, command)
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	if result.Reopened {
		events, err := s.acceptedRunEvents(ctx, result)
		return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: result.UserItemID}, events, err
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: result.UserItemID}, mapRunEvents(result.Events), nil
}

func runCommandIdentity(ctx context.Context) *runs.CommandIdentity {
	identity, ok := commandIdentityFrom(ctx)
	if !ok {
		return nil
	}
	return &runs.CommandIdentity{Key: identity.key, Fingerprint: identity.fingerprint}
}

// acceptedRunEvents observes a command's original opening.
// The acceptance owns the run identity; the live stream is re-attached by the same
// path a reconnect uses, honoring any replay cursor the retry carried. A run that
// is no longer attachable yields the identity with an empty stream so the client
// re-reads durable state instead of failing the retry.
func (s *Handler) acceptedRunEvents(ctx context.Context, accepted runs.StartResult) (iter.Seq2[protocol.RunEvent, error], error) {
	if accepted.MaterialRetired {
		return emptyRunEventStream, nil
	}
	_, events, err := s.SubscribeRun(ctx, protocol.SubscribeRunRequest{RunID: accepted.RunID, SegmentID: accepted.SegmentID})
	switch {
	case unattachable(err):
		return emptyRunEventStream, nil
	case err != nil:
		return nil, err
	default:
		return events, nil
	}
}

var emptyRunEventStream iter.Seq2[protocol.RunEvent, error] = func(func(protocol.RunEvent, error) bool) {}

func unattachable(err error) bool {
	return errors.Is(err, protocol.ErrRunNotFound) ||
		errors.Is(err, protocol.ErrRunWaiting) ||
		errors.Is(err, protocol.ErrRunFinished) ||
		errors.Is(err, protocol.ErrStaleSegment) ||
		errors.Is(err, protocol.ErrReplayCursorInvalid) ||
		errors.Is(err, protocol.ErrReplayUnavailable)
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
	return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, constraint), constraint.Error())
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
	case errors.Is(err, runs.ErrCommandKeyConflict):
		// A retried start whose key no longer pins the same request is the same
		// refusal the generic replay store gives every other keyed command.
		return protocol.NewFailure(errors.Join(protocol.ErrIdempotencyConflict, err), "idempotency key is already bound to another operation")
	case errors.Is(err, workspaceapp.ErrPromptSourceTooLarge):
		return wireWorkspaceError(err)
	case errors.Is(err, runs.ErrInputRequired):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), "input must contain a user text or image block")
	case modelchoice.IsInvalid(err):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrInvalidRunOptions):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrUnsupportedModelSelection):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrInvalidStart):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, session.ErrInvalid):
		return protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	case errors.Is(err, runs.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, runs.ErrInstallationChanged):
		return pluginChanged(err)
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	default:
		return err
	}
}

func pluginChanged(err error) error {
	return protocol.NewFailure(errors.Join(protocol.ErrPluginChanged, err), "a plugin installation changed while the run was being prepared; nothing was started")
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
