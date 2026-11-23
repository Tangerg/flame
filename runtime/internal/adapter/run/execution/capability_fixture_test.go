package execution

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type withoutComposition struct{}

func (withoutComposition) Pending([]plugins.ContributionBinding) bool { return false }
func (withoutComposition) RestorationFailure([]plugins.ContributionBinding) (bool, error) {
	return false, nil
}
func (withoutComposition) ResolveToolProposals(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (withoutComposition) ValidateToolProposals(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (withoutComposition) PrepareToolProposal(_ context.Context, _ []plugins.ContributionBinding, call chat.ToolCall) (chat.ToolCall, error) {
	return call, nil
}
func (withoutComposition) Activations(context.Context, []plugins.ContributionBinding) ([]plugin.Activation, error) {
	return nil, nil
}
func (withoutComposition) ResolveInput(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (withoutComposition) ValidateInput(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (withoutComposition) PrepareInput(_ context.Context, _ []plugins.ContributionBinding, _ flamemodel.Selection, request *chat.Request) (*chat.Request, error) {
	return request, nil
}
func (withoutComposition) ResolveContext(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (withoutComposition) ValidateContext(context.Context, []plugins.ContributionBinding) error {
	return nil
}
func (withoutComposition) PrepareContext(context.Context, []plugins.ContributionBinding, flamemodel.Selection, flamemodel.Use, *chat.Request, *chat.Message, []chat.Message) ([]chat.Message, error) {
	return nil, nil
}
func (withoutComposition) Resolve(context.Context) ([]plugins.ContributionBinding, error) {
	return nil, nil
}
func (withoutComposition) Validate(context.Context, []plugins.ContributionBinding) error { return nil }
func (withoutComposition) HeldDependencies() []plugin.Dependency                         { return nil }
func (withoutComposition) Bind(model chat.Model, streamer chat.Streamer, _ []plugins.ContributionBinding) (chat.Model, chat.Streamer) {
	return model, streamer
}

type withoutOriginalToolResults struct{}

func (withoutOriginalToolResults) OwnsToolSource(context.Context, domaintool.Ref, framevalue.Digest, plugin.Activation) (bool, error) {
	return false, nil
}
func (withoutOriginalToolResults) AwaitOriginalToolSource(context.Context, domaintool.Ref, framevalue.Digest, framevalue.Digest, plugin.Activation) (bool, error) {
	return false, nil
}
func (withoutOriginalToolResults) OriginalToolResult(context.Context, domaintool.Ref, framevalue.Digest, framevalue.Digest, plugin.Activation, string, domaintool.Arguments) (*toolcontract.Output, error) {
	return nil, nil
}

type withoutHooks struct{}

func (withoutHooks) BeforeToolUse(context.Context, InteractionToolHookInput) (InteractionToolHookDecision, error) {
	return allowToolHook(false, nil), nil
}
func (withoutHooks) AfterToolUse(context.Context, InteractionToolHookInput) error { return nil }
func (withoutHooks) BeforeCompaction(context.Context, identity.SessionID, string) (bool, error) {
	return true, nil
}
func (withoutHooks) NotifyWaiting(context.Context, identity.SessionID, string) error { return nil }
func (withoutHooks) NotifyStopped(context.Context, identity.SessionID, string, string) error {
	return nil
}

type withoutMaintenance struct{}

func (withoutMaintenance) Maintain(context.Context, RunMaintenanceInput) RunMaintenanceResult {
	return RunMaintenanceResult{}
}

type uncompactedContext struct{}

func (uncompactedContext) CompactModelContext(_ context.Context, request ModelContextCompaction) (ModelContextCompactionResult, error) {
	candidate := request.Candidate()
	return NewModelContextCompactionResult(candidate, false, "", len(candidate), len(candidate))
}

func (uncompactedContext) CurrentSessionState(context.Context, identity.SessionID) ([]chat.Message, error) {
	return nil, nil
}

// Capabilities a test does not exercise get collaborators that contribute
// nothing, as an installation without them would.
func withRequiredCapabilities(config InteractionExecutorConfig) InteractionExecutorConfig {
	if config.CallComposition == nil {
		config.CallComposition = withoutComposition{}
	}
	if config.OriginalToolResults == nil {
		config.OriginalToolResults = withoutOriginalToolResults{}
	}
	if config.ToolResolver == nil {
		config.ToolResolver = staticInteractionTools{}
	}
	if config.ToolInterpreter == nil {
		config.ToolInterpreter = testInteractionToolInterpreter{}
	}
	if config.ToolPresenter == nil {
		config.ToolPresenter = toolset.Presenter{}
	}
	if config.ToolAuthorizer == nil {
		config.ToolAuthorizer = allowInteractionTools{}
	}
	if config.ToolHooks == nil {
		config.ToolHooks = withoutHooks{}
	}
	if config.LifecycleHooks == nil {
		config.LifecycleHooks = withoutHooks{}
	}
	if config.Maintenance == nil {
		config.Maintenance = withoutMaintenance{}
	}
	if config.ModelContextCompactor == nil {
		config.ModelContextCompactor = uncompactedContext{}
	}
	if config.ModelContextState == nil {
		config.ModelContextState = uncompactedContext{}
	}
	return config
}
