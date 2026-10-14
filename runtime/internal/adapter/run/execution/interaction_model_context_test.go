package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionContextContributionsAreMeasuredButNeverAdoptedAsConversation(t *testing.T) {
	for _, compact := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "capacity"}[compact], func(t *testing.T) {
			owner := &contextCaptureComposition{t: t}
			compactor := &contributedContextCompactor{t: t}
			calls := 0
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls++
				if countContextMaterial(request.Messages) != 1 {
					t.Errorf("call %d lost or duplicated contributed context: %+v", calls, request.Messages)
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
				cfg.ModelContextCompactor = compactor
				cfg.ModelContextState = emptyInteractionModelContextState{}
			}
			executor := newObservedTestInteractionExecutor(t, model, cfg)
			runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			if calls != 2 || owner.calls != 2 || (compact && compactor.calls != 2) {
				t.Fatalf("expected two complete model admissions: provider=%d context=%d capacity=%d", calls, owner.calls, compactor.calls)
			}
		})
	}
}

func TestInteractionContextFailureRefusesBeforeProviderDispatch(t *testing.T) {
	owner := &contextCaptureComposition{t: t, failure: errors.New("retrieval failed")}
	calls := 0
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("unexpected", 1, 1), nil
	}), InteractionExecutorConfig{ModelComposition: owner})
	runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	if calls != 0 || owner.calls != 1 {
		t.Fatalf("failed context dispatched model: provider=%d context=%d", calls, owner.calls)
	}
}

type contextCaptureComposition struct {
	t        *testing.T
	calls    int
	resolved int
	failure  error
}

func (c *contextCaptureComposition) ResolveContext(context.Context) ([]plugins.ContributionBinding, error) {
	c.resolved++
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		return nil, err
	}
	server := testsupport.ServerName("retrieval")
	origin, err := mcpserver.InstallationOrigin(dependedInstallation)
	if err != nil {
		return nil, err
	}
	id, err := mcpserver.NewID(origin, server)
	if err != nil {
		return nil, err
	}
	return []plugins.ContributionBinding{{InstallationID: dependedInstallation, Digest: testsupport.Digest("context release"), ContributionID: "retrieval", ServerID: id, ServerName: server, Export: "retrieval", Contract: contract.Identity()}}, nil
}
func (*contextCaptureComposition) ValidateContext(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (c *contextCaptureComposition) PrepareContext(ctx context.Context, bindings []plugins.ContributionBinding, selection modelref.Selection, request *chat.Request) ([]chat.Message, error) {
	c.calls++
	if executionctx.ModelCallsFromContext(ctx) == nil {
		c.t.Error("context preparation has no original execution publisher")
	}
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Parent == "" || invocation.Target.Reference != invocation.ID || selection != testDefaultSelection() || len(bindings) != 1 {
		c.t.Errorf("original model/context attribution changed: %+v %v", invocation, err)
	}
	if countContextMaterial(request.Messages) != 0 {
		c.t.Error("previous ephemeral contribution entered next conversation")
	}
	if c.failure != nil {
		return nil, c.failure
	}
	return []chat.Message{chat.NewSystemMessage("ephemeral retrieval")}, nil
}
func (*contextCaptureComposition) Resolve(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (*contextCaptureComposition) Validate(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (*contextCaptureComposition) HeldDependencies() []plugin.Dependency { return nil }
func (*contextCaptureComposition) Bind(model chat.Model, stream chat.Streamer, _ []plugins.ContributionBinding) (chat.Model, chat.Streamer) {
	return model, stream
}

type contributedContextCompactor struct {
	t     *testing.T
	calls int
}

func (c *contributedContextCompactor) CompactModelContext(_ context.Context, request ModelContextCompaction) (ModelContextCompactionResult, error) {
	c.calls++
	if countContextMaterial(request.Instructions()) != 1 || countContextMaterial(request.Candidate()) != 0 {
		c.t.Error("capacity boundary lost ephemeral context ownership")
	}
	return NewModelContextCompactionResult(request.Candidate(), false, "", len(request.Candidate()), 100)
}
func countContextMaterial(messages []chat.Message) int {
	count := 0
	for _, m := range messages {
		if m.Text() == "ephemeral retrieval" {
			count++
		}
	}
	return count
}

func (*contextCaptureComposition) ResolveInput(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (*contextCaptureComposition) ValidateInput(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (*contextCaptureComposition) PrepareInput(_ context.Context, _ []plugins.ContributionBinding, _ modelref.Selection, request *chat.Request) (*chat.Request, error) {
	return request.Clone(), nil
}
