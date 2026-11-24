package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"

	framehost "github.com/Tangerg/frame/host"
	corechat "github.com/Tangerg/scope/core/chat"
)

func (s *Handler) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	sessionID, parseErr := optionalSessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, nil, parseErr
	}
	options := generationOptionsFromWire(in.Params)
	selection, err := flamemodel.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	input, err := decodeRunInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	capabilities, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	command := runs.StartCommand{
		SessionID:      sessionID,
		ModelSelection: selection,
		Options:        options,
		Capabilities:   capabilities,
		Input:          input,
		Acceptance:     runCommandIdentity(ctx),
	}
	if in.NewSession != nil {
		runID, err := wireIdentity("newSession.runId", in.NewSession.RunID, identity.ParseRun)
		if err != nil {
			return nil, nil, err
		}
		sessionID, err := wireIdentity("newSession.sessionId", in.NewSession.SessionID, identity.ParseSession)
		if err != nil {
			return nil, nil, err
		}
		project, err := wireIdentity("newSession.projectId", in.NewSession.ProjectID, identity.ParseOptionalProject)
		if err != nil {
			return nil, nil, err
		}
		command.NewSession = &runs.SessionOpening{SessionID: sessionID, RunID: runID, Title: in.NewSession.Title, CWD: s.sessionWorkspacePath(in.NewSession.Workspace), Project: project}
	}
	if actor, present := operation.ActorFrom(ctx); present {
		command.Actor = &actor
	}
	result, err := s.runOpenings.Start(ctx, command)
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	if result.Reopened() {
		events, err := s.acceptedRunEvents(ctx, result)
		return &protocol.StartRunResponse{RunID: result.RunID.String(), SegmentID: result.SegmentID.String(), UserItemID: result.UserItemID.String()}, events, err
	}
	return &protocol.StartRunResponse{RunID: result.RunID.String(), SegmentID: result.SegmentID.String(), UserItemID: result.UserItemID.String()}, mapRunEvents(result.Events), nil
}

func runCommandIdentity(ctx context.Context) *runs.CommandIdentity {
	identity, ok := commandIdentityFrom(ctx)
	if !ok {
		return nil
	}
	return &runs.CommandIdentity{Key: identity.key, Fingerprint: identity.fingerprint}
}

func (s *Handler) acceptedRunEvents(ctx context.Context, accepted runs.StartResult) (iter.Seq2[protocol.RunEvent, error], error) {
	if accepted.Opening == runs.OpeningRetired {
		return emptyRunEventStream, nil
	}
	_, events, err := s.SubscribeRun(ctx, protocol.SubscribeRunRequest{RunID: accepted.RunID.String(), SegmentID: accepted.SegmentID.String()})
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
	return protocol.NewFailure(protocol.ErrInvalidParams, constraint.Error(), constraint)
}

func wireRunStartErr(err error) error {
	if conflict, ok := errors.AsType[*runs.ActiveRunConflictError](err); ok {
		return &ActiveRunConflictError{ActiveRun: protocol.ActiveRunRef{
			RunID: conflict.RunID.String(), Status: presentRunStatus(conflict.State),
		}}
	}
	switch {
	case errors.Is(err, runs.ErrCommandKeyConflict):
		return protocol.NewFailure(protocol.ErrIdempotencyConflict, errIdempotencyRebound.Error(), err)
	case errors.Is(err, workspaceapp.ErrPromptSourceTooLarge):
		return wireWorkspaceError(err)
	case errors.Is(err, runs.ErrInputRequired):
		return protocol.NewFailure(protocol.ErrInvalidParams, "input must contain a user text or image block", err)
	case flamemodel.IsInvalid(err):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, runs.ErrInvalidRunOptions):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, runs.ErrUnsupportedModelSelection):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, runs.ErrInvalidStart):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, session.ErrInvalid):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	case errors.Is(err, rundomain.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, runs.ErrInstallationChanged), errors.Is(err, framehost.ErrStaleBinding):
		return pluginChanged(err)
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	case errors.Is(err, project.ErrNotFound):
		return protocol.ErrProjectNotFound
	default:
		return mapModelError(err)
	}
}

func pluginChanged(err error) error {
	return protocol.NewFailure(protocol.ErrPluginChanged, "a plugin installation changed while the run was being prepared; nothing was started", err)
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
