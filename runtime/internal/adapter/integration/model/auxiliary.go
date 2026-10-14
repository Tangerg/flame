package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/modeltext"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/flame/runtime/model/catalog"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// AuxiliaryResolver selects the current utility-role model for each call.
// Resolving at the boundary lets a role configuration change take effect without
// rebuilding the owning worker. The caller releases the frozen capture after
// all calls have joined; retained observers cannot retire before that boundary.
type AuxiliaryResolver func(context.Context, AuxiliaryBudget) (SelectedChat, func(), error)

// AuxiliaryBudget is the caller-owned input/output envelope for a captured use
// case. Every complete model-visible request must remain within it.
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

// SelectedChat captures the exact selection and model construction
// together. Wrapping its model cannot detach or invent the selected identity.
type SelectedChat struct {
	selection modelchoice.Selection
	resolved  ResolvedChat
	limits    modelref.TokenLimits
}

func NewSelectedChat(selection modelchoice.Selection, resolved ResolvedChat) (SelectedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return SelectedChat{}, err
	}
	if dependency.Missing(resolved.Model()) {
		return SelectedChat{}, errors.New("auxiliary model: resolved model is required")
	}
	limits, _, err := LookupTokenLimits(selection)
	if err != nil {
		return SelectedChat{}, err
	}
	return SelectedChat{selection: selection, resolved: resolved, limits: limits}, nil
}
func (m SelectedChat) Selection() modelchoice.Selection { return m.selection }
func (m SelectedChat) Model() chat.Model                { return m.resolved.Model() }
func (m SelectedChat) InputTokenCounter() (InputTokenCounter, bool) {
	return m.resolved.InputTokenCounter()
}

// AuxiliaryPrompt is the complete resource envelope for one auxiliary model request.
// Input bytes and output tokens are deliberately mandatory: background
// maintenance must never inherit a provider's context/output defaults.
type AuxiliaryPrompt struct {
	Operation       string
	SystemPrompt    string
	UserPrompt      string
	MaxInputBytes   int
	MaxOutputTokens int64
}

func (p AuxiliaryPrompt) validate() error {
	if strings.TrimSpace(p.Operation) == "" {
		return errors.New("auxiliary model: operation is required")
	}
	if err := (AuxiliaryBudget{MaxInputBytes: p.MaxInputBytes, MaxOutputTokens: p.MaxOutputTokens}).Validate(); err != nil {
		return err
	}
	inputBytes := len(p.SystemPrompt) + len(p.UserPrompt)
	if inputBytes > p.MaxInputBytes {
		return fmt.Errorf(
			"auxiliary model: prompt is %d bytes; input limit is %d",
			inputBytes,
			p.MaxInputBytes,
		)
	}
	return nil
}

// Complete resolves the live auxiliary selection and returns a complete text
// generation inside the caller's resource envelope. A rejected or incomplete
// generation cannot become a durable summary, memory, or skill proposal.
func (r AuxiliaryResolver) Complete(ctx context.Context, prompt AuxiliaryPrompt) (text string, err error) {
	if r == nil {
		return "", errors.New("auxiliary model: resolver is required")
	}
	if err := prompt.validate(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	invocation := capability.Invocation{ID: id, Origin: capability.Automatic, Parent: operation.Identity(ctx), Target: capability.Target{Scope: "call", Reference: id}}
	if parent, err := pluginsdk.InvocationFromContext(ctx); err == nil {
		invocation.Origin, invocation.Parent = parent.Origin, parent.ID
	}
	ctx, err = pluginsdk.WithInvocation(ctx, invocation)
	if err != nil {
		return "", err
	}
	// The span carries what Scope cannot see: which product operation asked and
	// how much input it spent. Request, usage, and finish-reason telemetry
	// belongs to the model span Scope's middleware opens at the provider
	// boundary, so this one does not record those facts a second time.
	ctx, span := otel.Tracer("scope/flame/adapter/integration/model").Start(ctx, "auxiliary model",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("auxiliary.operation", prompt.Operation),
			attribute.Int("auxiliary.input_bytes", len(prompt.SystemPrompt)+len(prompt.UserPrompt)),
		),
	)
	stage := "resolve"
	defer func() {
		if err != nil {
			// An unusable completion is the model answering, not the call
			// failing: Scope's output contract is what separates them.
			if stage == "call" && errors.Is(err, chatclient.ErrInvalidOutput) {
				stage = "response"
			}
			span.SetStatus(codes.Error, "auxiliary model failed")
			span.SetAttributes(attribute.String("auxiliary.failure_stage", stage))
			if errors.Is(err, context.Canceled) {
				span.SetAttributes(attribute.String("error.type", "canceled"))
			} else if errors.Is(err, context.DeadlineExceeded) {
				span.SetAttributes(attribute.String("error.type", "deadline_exceeded"))
			}
		}
		span.End()
	}()
	captured, release, err := r(ctx, AuxiliaryBudget{MaxInputBytes: prompt.MaxInputBytes, MaxOutputTokens: prompt.MaxOutputTokens})
	if err != nil {
		return "", err
	}
	if release == nil {
		return "", errors.New("auxiliary model: captured model release is required")
	}
	defer release()
	if dependency.Missing(captured.Model()) {
		return "", errors.New("auxiliary model: model is required")
	}
	client, err := chatclient.New(captured.Model(), chatclient.Config{})
	if err != nil {
		return "", err
	}
	stage = "call"
	text, err = client.Output(ctx, &chat.Request{Messages: []chat.Message{
		chat.NewSystemMessage(prompt.SystemPrompt),
		chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt)),
	}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}}, chatclient.Text())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: auxiliary model returned blank text", chatclient.ErrInvalidOutput)
	}
	return text, nil
}

// AdmitRequest applies the selected model's known input/output envelope to the
// complete request after context composition. A catalog miss retains unknown
// capacity; it cannot manufacture a limit from the caller's byte budget.
func (m SelectedChat) AdmitRequest(ctx context.Context, request *chat.Request) error {
	if request == nil {
		return errors.New("auxiliary model: request is required")
	}
	if err := m.AdmitInput(request); err != nil {
		return err
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
		estimated, err := modeltext.EstimateRequestTokens(request)
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

// WithModel retains the frozen selected identity, capacity and counter while
// replacing only its call projection with a caller-owned decorator.
func (m SelectedChat) WithModel(model chat.Model) (SelectedChat, error) {
	counter, _ := m.InputTokenCounter()
	resolved, err := NewResolvedChat(model, counter)
	if err != nil {
		return SelectedChat{}, err
	}
	m.resolved = resolved
	return m, nil
}

// PrepareRequest applies model-owned options before contributors and capacity
// observe the same final request. Callers cannot replace a captured selection.
func (m SelectedChat) PrepareRequest(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, errors.New("auxiliary model: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if err := m.selection.ValidateExact(); err != nil {
		return nil, err
	}
	if request.Options.Model != "" && request.Options.Model != m.selection.Model() {
		return nil, errors.New("model: request cannot replace selected model")
	}
	effort := chat.ReasoningEffort(m.selection.ReasoningEffort())
	if request.Options.ReasoningEffort != "" && request.Options.ReasoningEffort != effort {
		return nil, errors.New("model: request cannot replace selected reasoning effort")
	}
	prepared := request.Clone()
	prepared.Options.ReasoningEffort = effort
	return prepared, nil
}

// AdmitInput validates the model-visible projection, never the authored source
// that an input contributor may legitimately convert before model execution.
func (m SelectedChat) AdmitInput(request *chat.Request) error {
	if request == nil {
		return errors.New("model: final request is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := m.selection.ValidateExact(); err != nil {
		return err
	}
	if err := (modelcatalog.Selections{}).AdmitInput(m.selection, request.Messages); err != nil {
		return err
	}
	if request.Options.Model != "" && request.Options.Model != m.selection.Model() {
		return errors.New("model: request cannot replace selected model")
	}
	return nil
}
