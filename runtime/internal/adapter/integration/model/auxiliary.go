package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	coreinvocation "github.com/Tangerg/scope/core/invocation"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type AuxiliaryResolver func(context.Context, AuxiliaryBudget) (SelectedChat, func() error, error)

type AuxiliaryBudget struct {
	MaxInputBytes   int
	MaxOutputTokens int64
}

func (b AuxiliaryBudget) Validate() error {
	if b.MaxInputBytes <= 0 || b.MaxOutputTokens <= 0 {
		return errors.New("auxiliary model: positive input/output budget is required")
	}
	return nil
}

type SelectedChat struct {
	admission catalog.Admission
	resolved  ResolvedChat
	limits    modelref.TokenLimits
}

func NewSelectedChat(selection flamemodel.Selection, resolved ResolvedChat) (SelectedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return SelectedChat{}, err
	}
	if lo.IsNil(resolved.Model()) {
		return SelectedChat{}, errors.New("auxiliary model: resolved model is required")
	}
	admission, err := catalog.NewAdmission(selection, resolved.metadata)
	if err != nil {
		return SelectedChat{}, err
	}
	var limits modelref.TokenLimits
	if resolved.metadata != nil {
		limits, err = catalogTokenLimits(*resolved.metadata)
		if err != nil {
			return SelectedChat{}, err
		}
	}
	return SelectedChat{admission: admission, resolved: resolved, limits: limits}, nil
}

func (m SelectedChat) Selection() flamemodel.Selection   { return m.admission.Selection() }
func (m SelectedChat) Model() chat.Model                 { return m.resolved.Model() }
func (m SelectedChat) TokenLimits() modelref.TokenLimits { return m.limits }
func (m SelectedChat) Capture() (ProviderCapture, bool)  { return m.resolved.Capture() }
func (m SelectedChat) Close(ctx context.Context) error   { return m.resolved.Close(ctx) }

func (m SelectedChat) RequestFits(ctx context.Context, request *chat.Request) (bool, error) {
	return m.resolved.fits(ctx, request)
}

func (m SelectedChat) InputTokenCounter() (InputTokenCounter, bool) {
	return m.resolved.InputTokenCounter()
}

type AuxiliaryOperation string

const (
	AuxiliaryTitle      AuxiliaryOperation = "title"
	AuxiliaryCompaction AuxiliaryOperation = "compaction"
)

// Input bytes and output tokens are mandatory: background maintenance must never
// inherit a provider's context or output defaults.
type AuxiliaryPrompt struct {
	Operation    AuxiliaryOperation
	SystemPrompt string
	UserPrompt   string
	Budget       AuxiliaryBudget
}

func (p AuxiliaryPrompt) inputBytes() int { return len(p.SystemPrompt) + len(p.UserPrompt) }

func (p AuxiliaryPrompt) validate() error {
	switch p.Operation {
	case AuxiliaryTitle, AuxiliaryCompaction:
	default:
		return fmt.Errorf("auxiliary model: unknown operation %q", p.Operation)
	}
	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if p.inputBytes() > p.Budget.MaxInputBytes {
		return fmt.Errorf(
			"auxiliary model: prompt is %d bytes; input limit is %d",
			p.inputBytes(),
			p.Budget.MaxInputBytes,
		)
	}
	return nil
}

type auxiliaryFailureStage string

const (
	failedResolving auxiliaryFailureStage = "resolve"
	failedCalling   auxiliaryFailureStage = "call"
	failedResponse  auxiliaryFailureStage = "response"
)

func recordAuxiliaryFailure(span trace.Span, stage auxiliaryFailureStage, err error) {
	if stage == failedCalling && errors.Is(err, chatclient.ErrInvalidOutput) {
		stage = failedResponse
	}
	span.SetStatus(codes.Error, "auxiliary model failed")
	span.SetAttributes(attribute.String("auxiliary.failure_stage", string(stage)))
	switch {
	case errors.Is(err, context.Canceled):
		span.SetAttributes(telemetry.ErrorTypeKey.String("canceled"))
	case errors.Is(err, context.DeadlineExceeded):
		span.SetAttributes(telemetry.ErrorTypeKey.String("deadline_exceeded"))
	}
}

func (r AuxiliaryResolver) Complete(ctx context.Context, prompt AuxiliaryPrompt) (text string, err error) {
	if r == nil {
		return "", errors.New("auxiliary model: resolver is required")
	}
	if err := prompt.validate(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	invocation := framecall.Invocation{ID: id, Origin: extension.Automatic, Parent: operation.Identity(ctx), Target: framecall.Target{Scope: extension.ScopeCall, Reference: id}}
	if parent, err := pluginmcp.InvocationFromContext(ctx); err == nil {
		invocation.Origin, invocation.Parent = parent.Origin, parent.ID
	}
	ctx, err = pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		return "", err
	}
	// Request, usage and finish-reason telemetry belong to Scope's model span, so this
	// span records only the product operation and the input it spent.
	ctx, span := otel.Tracer("github.com/Tangerg/flame/runtime/internal/adapter/integration/model").Start(ctx, "auxiliary model",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("auxiliary.operation", string(prompt.Operation)),
			attribute.Int("auxiliary.input_bytes", prompt.inputBytes()),
		),
	)
	stage := failedResolving
	defer func() {
		if err != nil {
			recordAuxiliaryFailure(span, stage, err)
		}
		span.End()
	}()
	captured, release, err := r(ctx, prompt.Budget)
	if err != nil {
		return "", err
	}
	if release == nil {
		return "", errors.New("auxiliary model: captured model release is required")
	}
	defer func() { err = errors.Join(err, release()) }()
	if lo.IsNil(captured.Model()) {
		return "", errors.New("auxiliary model: model is required")
	}
	client, err := chatclient.New(captured.Model())
	if err != nil {
		return "", err
	}
	stage = failedCalling
	text, err = client.Output(ctx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxOutputTokens: &prompt.Budget.MaxOutputTokens}}, chatclient.Text())
	if err != nil {
		if _, complete := errors.AsType[*chatclient.OutputCompletionError](err); complete {
			err = &coreinvocation.Failure{Kind: coreinvocation.FailureFailed, Cause: err}
		}
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: auxiliary model returned blank text", chatclient.ErrInvalidOutput)
	}
	return text, nil
}

func (m SelectedChat) AdmitRequest(ctx context.Context, request *chat.Request) error {
	if request == nil {
		return errors.New("auxiliary model: request is required")
	}
	if err := m.AdmitInput(request); err != nil {
		return err
	}
	fits, err := m.RequestFits(ctx, request)
	if err != nil {
		return err
	}
	if !fits {
		return errors.New("model: complete request exceeds the installed transport capacity")
	}
	reservation := modelref.OutputReservation{}
	if request.Options.MaxOutputTokens != nil {
		var err error
		reservation, err = modelref.NewOutputReservation(*request.Options.MaxOutputTokens)
		if err != nil {
			return err
		}
	}
	ceiling, known, err := m.limits.InputCeiling(reservation)
	if err != nil {
		return err
	}
	if !known {
		return nil
	}
	measurement := "estimated input"
	var count int64
	if counter, found := m.InputTokenCounter(); found {
		measurement = "provider input"
		count, err = counter.CountInputTokens(ctx, request.Clone())
		if err != nil {
			return fmt.Errorf("auxiliary model: count selected provider input: %w", err)
		}
		if count <= 0 {
			return errors.New("auxiliary model: selected provider returned a non-positive input count")
		}
	} else {
		estimated, err := flamemodel.EstimateRequestTokens(request)
		if err != nil {
			return err
		}
		count = int64(estimated)
	}
	if count > ceiling {
		return fmt.Errorf("auxiliary model: %s requires %d tokens; selected model permits %d after output reservation", measurement, count, ceiling)
	}
	return nil
}

func (m SelectedChat) WithModel(model chat.Model) (SelectedChat, error) {
	resolved, err := m.resolved.WithModel(model)
	if err != nil {
		return SelectedChat{}, err
	}
	m.resolved = resolved
	return m, nil
}

func (m SelectedChat) PrepareRequest(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, errors.New("auxiliary model: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if err := m.Selection().ValidateExact(); err != nil {
		return nil, err
	}
	if request.Options.Model != "" && request.Options.Model != m.Selection().Model() {
		return nil, errors.New("model: request cannot replace selected model")
	}
	effort := chat.ReasoningEffort(m.Selection().ReasoningEffort())
	if request.Options.ReasoningEffort != "" && request.Options.ReasoningEffort != effort {
		return nil, errors.New("model: request cannot replace selected reasoning effort")
	}
	prepared := request.Clone()
	prepared.Options.ReasoningEffort = effort
	return prepared, nil
}

func (m SelectedChat) AdmitInput(request *chat.Request) error {
	if request == nil {
		return errors.New("model: final request is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := m.Selection().ValidateExact(); err != nil {
		return err
	}
	if err := m.admission.AdmitInput(request.Messages); err != nil {
		return err
	}
	if request.Options.Model != "" && request.Options.Model != m.Selection().Model() {
		return errors.New("model: request cannot replace selected model")
	}
	return nil
}
