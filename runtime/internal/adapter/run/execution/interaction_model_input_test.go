package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type inputCaptureComposition struct {
	contextCaptureComposition
	transformed       int
	failure           error
	expectedSelection modelref.Selection
}

func (*inputCaptureComposition) ResolveContext(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (c *inputCaptureComposition) ResolveInput(ctx context.Context) ([]plugins.ContributionBinding, error) {
	bindings, err := c.contextCaptureComposition.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	bindings[0].ContributionID = "transform"
	bindings[0].Export = "transform"
	bindings[0].Contract = contract.Identity()
	return bindings, nil
}
func (c *inputCaptureComposition) PrepareInput(ctx context.Context, bindings []plugins.ContributionBinding, selection modelref.Selection, request *chat.Request) (*chat.Request, error) {
	c.transformed++
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Target.Reference != invocation.ID || invocation.Parent == "" || len(bindings) != 1 || (selection != testDefaultSelection() && selection != c.expectedSelection) {
		c.t.Errorf("original primary transform attribution changed: %+v %v", invocation, err)
	}
	if c.failure != nil {
		return nil, c.failure
	}
	result := request.Clone()
	for index, message := range result.Messages {
		if strings.Contains(message.Text(), " changed") {
			c.t.Error("previous transform entered retained conversation")
		}
		if message.Role == chat.RoleUser {
			result.Messages[index] = chat.NewUserMessage(chat.NewTextPart(message.Text() + " changed"))
		}
	}
	result.Options.Temperature = new(0.27)
	return result, nil
}
func TestPrimaryInputTransformationIsFinalRequestOnlyAcrossSuccessiveCalls(t *testing.T) {
	for _, compact := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "compacted"}[compact], func(t *testing.T) {
			owner := &inputCaptureComposition{contextCaptureComposition: contextCaptureComposition{t: t}}
			calls := 0
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls++
				if request.Options.Temperature == nil || *request.Options.Temperature != 0.27 || request.Messages[0].Text() != "current question changed" {
					t.Errorf("provider did not receive complete transformed request: %+v", request)
				}
				if calls == 1 {
					return interactionToolResponse(chat.ToolCall{ID: "echo-call", Name: "echo", Arguments: `{}`}, 5, 1), nil
				}
				return interactionUsageTextResponse("done", 7, 1), nil
			})
			echo, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "echo", Description: "Echo."}, func(context.Context, struct{}) (string, error) { return "echo", nil })
			if err != nil {
				t.Fatal(err)
			}
			cfg := InteractionExecutorConfig{ModelComposition: owner, ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "echo")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{echo}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}}
			if compact {
				cfg.ModelContextCompactor = &calibrationCaptureCompactor{estimatedTokens: 100}
				cfg.ModelContextState = emptyInteractionModelContextState{}
			}
			executor := newObservedTestInteractionExecutor(t, model, cfg)
			runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			if calls != 2 || owner.transformed != 2 || owner.resolved != 1 {
				t.Fatalf("primary transform capture/calls: provider=%d transform=%d resolve=%d", calls, owner.transformed, owner.resolved)
			}
		})
	}
}
func TestPrimaryInputFailureRefusesBeforeProviderDispatch(t *testing.T) {
	owner := &inputCaptureComposition{contextCaptureComposition: contextCaptureComposition{t: t}, failure: errors.New("transform failed")}
	calls := 0
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("unexpected", 1, 1), nil
	}), InteractionExecutorConfig{ModelComposition: owner})
	runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	if calls != 0 || owner.transformed != 1 {
		t.Fatalf("failed primary transform dispatched: %d/%d", calls, owner.transformed)
	}
}

type waitingInputComposition struct {
	*contextCaptureComposition
	inputResolves int
	inputCalls    int
}

func (c *waitingInputComposition) ResolveInput(ctx context.Context) ([]plugins.ContributionBinding, error) {
	c.inputResolves++
	owner := &inputCaptureComposition{contextCaptureComposition: contextCaptureComposition{t: c.t}}
	return owner.ResolveInput(ctx)
}
func (c *waitingInputComposition) PrepareInput(_ context.Context, _ []plugins.ContributionBinding, _ modelref.Selection, request *chat.Request) (*chat.Request, error) {
	c.inputCalls++
	if len(request.Options.Stop) != 0 {
		c.t.Error("transformed Options entered retained context")
	}
	result := request.Clone()
	result.Options.Stop = []string{"TRANSFORMED"}
	return result, nil
}

type primaryInputCounterModel struct {
	t      *testing.T
	count  int64
	calls  int
	counts int
}

func (m *primaryInputCounterModel) CountInputTokens(_ context.Context, request *chat.Request) (int64, error) {
	m.counts++
	if request.Messages[0].Text() != "current question changed" || request.Options.Temperature == nil || *request.Options.Temperature != 0.27 {
		m.t.Error("final primary capacity counted original input")
	}
	return m.count, nil
}
func (m *primaryInputCounterModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.calls++
	if request.Messages[0].Text() != "current question changed" {
		m.t.Error("counted input differs from provider input")
	}
	return interactionUsageTextResponse("done", 7, 1), nil
}
func TestPrimaryFinalCapacityCountsTransformedInputBeforeProvider(t *testing.T) {
	for _, count := range []int64{272000, 272001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			selected := testsupport.MustModelSelection("openai", "gpt-5-pro")
			owner := &inputCaptureComposition{contextCaptureComposition: contextCaptureComposition{t: t}, expectedSelection: selected}
			model := &primaryInputCounterModel{t: t, count: count}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ModelComposition: owner})
			start := interactionTestStart()
			start.ModelSelection = selected
			start.Options = &chat.Options{MaxOutputTokens: new(int64(64))}
			runInteractionHarness(t.Context(), t, executor, start, nil)
			want := 0
			if count == 272000 {
				want = 1
			}
			if model.calls != want || model.counts != 1 {
				t.Fatalf("transformed input capacity: generation=%d count=%d", model.calls, model.counts)
			}
		})
	}
}
