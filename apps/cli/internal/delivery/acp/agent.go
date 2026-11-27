package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/acp"
	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type Runtime interface {
	runworkflow.Runtime
	CreateSession(context.Context, conversation.CreateSession) (conversation.Session, error)
	ListSessions(context.Context, conversation.SessionQuery) (conversation.SessionPage, error)
	Resolve(context.Context, workspace.ResolveRequest) (workspace.Workspace, error)
}

type agent struct {
	runtime     Runtime
	options     authoring.RunOptions
	replay      mutation.ReplayPolicy
	diagnostics io.Writer
}

func Serve(ctx context.Context, transport acp.Transport, runtime Runtime, options authoring.RunOptions, replay mutation.ReplayPolicy, version string, diagnostics io.Writer) error {
	handler := agent{runtime: runtime, options: options, replay: replay, diagnostics: diagnostics}
	server, err := acp.NewAgent(&acp.AgentConfig{
		Info:          &acp.Implementation{Name: "flame", Title: acp.OptValue("Flame"), Version: version},
		NewSession:    handler.newSession,
		ListSessions:  handler.listSessions,
		LoadSession:   handler.loadSession,
		ResumeSession: handler.resumeSession,
		Prompt:        handler.prompt,
	})
	if err != nil {
		return fmt.Errorf("configure acp agent: %w", err)
	}
	return server.Run(ctx, transport)
}

func (a agent) prompt(ctx context.Context, session *acp.AgentSession, request *acp.PromptRequest) (*acp.PromptResponse, error) {
	message, err := promptMessage(request.Prompt)
	if err != nil {
		return nil, err
	}
	id := string(session.ID())
	if err := protocol.ValidateSessionID(id); err != nil {
		return nil, invalidParams("invalid session identifier")
	}
	turn, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	view := newProjection(context.WithoutCancel(turn), session)
	err = runworkflow.Execute(turn, runworkflow.Invocation{
		Runtime: a.runtime, Renderer: view, ReplayPolicy: a.replay,
		Start: authoring.StartRun{SessionID: id, Message: message, Options: a.options},
		AnswerInterrupts: func(ctx context.Context, interrupts []conversation.Interrupt) ([]conversation.InterruptAnswer, error) {
			answers, err := view.answerInterrupts(ctx, interrupts)
			if errors.Is(err, errPermissionCancelled) {
				cancel(err)
			}
			return answers, err
		},
	})
	if err == nil {
		return &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}
	if _, unconfirmed := errors.AsType[*runworkflow.CancellationError](err); unconfirmed {
		_, _ = fmt.Fprintln(a.diagnostics, "acp: runtime could not confirm cancellation for session", id)
		return nil, &acp.Error{Code: acp.ErrorCodeInternalError, Message: "runtime could not confirm cancellation"}
	}
	if errors.Is(context.Cause(turn), errPermissionCancelled) {
		return &acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if outcome, ok := errors.AsType[*runworkflow.OutcomeError](err); ok {
		switch outcome.Outcome.Status {
		case protocol.OutcomeCanceled:
			return &acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
		case protocol.OutcomeLost:
			if turn.Err() != nil {
				_, _ = fmt.Fprintln(a.diagnostics, "acp: run lost after cancelling session", id, "-", outcome.Outcome.Explanation())
			}
		}
	}
	return nil, runtimeError(err)
}

func promptMessage(blocks []acp.ContentBlock) (authoring.Message, error) {
	var text strings.Builder
	for index, block := range blocks {
		if index > 0 {
			text.WriteString("\n\n")
		}
		switch value := block.(type) {
		case *acp.TextContent:
			text.WriteString(value.Text)
		case *acp.ResourceLink:
			fmt.Fprintf(&text, "Resource %q: %q", value.Name, value.URI)
		default:
			return authoring.Message{}, invalidParams("unsupported prompt content")
		}
	}
	message := authoring.Message{Text: text.String()}
	if err := message.Validate(); err != nil {
		return authoring.Message{}, invalidParams(err.Error())
	}
	return message, nil
}

func invalidParams(message string) error {
	return &acp.Error{Code: acp.ErrorCodeInvalidParams, Message: message}
}

func runtimeError(err error) error {
	switch {
	case errors.Is(err, conversation.ErrSessionNotFound):
		return &acp.Error{Code: acp.ErrorCodeResourceNotFound, Message: "session not found"}
	case errors.Is(err, conversation.ErrSessionHasActiveRun), errors.Is(err, conversation.ErrSessionBusy):
		return &acp.Error{Code: acp.ErrorCodeInvalidRequest, Message: "session has pending work"}
	case errors.Is(err, errQuestionRequiresFlame):
		return &acp.Error{Code: acp.ErrorCodeInvalidRequest, Message: errQuestionRequiresFlame.Error()}
	case errors.Is(err, conversation.ErrReplayCursorInvalid):
		return invalidParams("invalid or expired Flame session cursor")
	case errors.Is(err, protocol.ErrInvalidParams):
		return invalidParams("runtime rejected the request parameters")
	case errors.Is(err, context.Canceled):
		return &acp.Error{Code: acp.ErrorCodeRequestCancelled, Message: "request cancelled"}
	default:
		if outcome, ok := errors.AsType[*runworkflow.OutcomeError](err); ok {
			return &acp.Error{Code: acp.ErrorCodeInternalError, Message: "run " + string(outcome.Outcome.Status)}
		}
		return &acp.Error{Code: acp.ErrorCodeInternalError, Message: "runtime operation failed"}
	}
}
