package bootstrap

import (
	"context"
	"errors"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/scope/core/chat"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools           toolset.Built
	mcp             *mcp.Pool
	memory          *pluginservices.Memory
	toolResults     *pluginservices.ToolResults
	pluginCalls     *pluginservices.CommandCalls
	callComposition *pluginservices.CallComposition
	modelCallbacks  []pluginsdk.Callback
	utilityModel    models.AuxiliaryResolver
	closers         []*teardownStep
	admission       *plugins.Admission
}

// toolEnvironmentDependencies is the complete construction contract for the
// process-owned tool runtime. Keeping the contract as one value lets construction
// transfer every acquisition to runtimeLifetime even when construction fails.
type toolEnvironmentDependencies struct {
	modelServices  modelEnvironment
	workspaceScope *workspace.Scope
	conversation   interface {
		Read(context.Context, string) ([]chat.Message, error)
	}
	publish        invalidation.Publish
	packageSkills  promptsource.PackageSkills
	registry       *plugins.Registry
	lifetime       context.Context
	config         assemblyConfig
	approvalPolicy *approvals.RuntimePolicy
	mcp            mcpEnvironment
	goalReader     *goals.Reader
	goalReporter   *goals.OutcomeReporter
	plan           *sessions.PlanCoordinator
	skillStore     *skillauthoring.Store
	skillProposals builtin.SkillProposalSubmitter
}

type toolEnvironmentBuilder func(context.Context, toolEnvironmentDependencies) (toolEnvironment, error)

func buildToolEnvironment(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
	cfg := deps.config
	prepareTool, err := pluginservices.NewToolPreparation(pluginservices.ToolPreparationConfig{Installations: cfg.stores.Installations, Callbacks: deps.modelServices.callbacks, Embeddings: deps.modelServices.liveEmbedder.Resolve, Invalidations: deps.publish})
	if err != nil {
		return toolEnvironment{}, err
	}
	admission := plugins.NewAdmission()
	commands, err := plugins.NewCommands(cfg.stores.PluginCommands, admission)
	if err != nil {
		return toolEnvironment{}, err
	}
	mcpPool, err := mcp.Open(ctx, deps.lifetime, deps.mcp.servers, cfg.stores.MCPAuthorization(), deps.registry, deps.modelServices.callbacks, prepareTool, deps.publish, commands)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	environment := toolEnvironment{
		mcp: mcpPool,
		// The SDK consumes each ClientSession transport closer even when Close
		// returns a diagnostic. Step owns the caller deadline; the action itself
		// must outlive that deadline and return only when the pool generation has
		// actually settled, so a timed-out Instance retains and later joins it.
		closers: []*teardownStep{terminalStep(func(ctx context.Context) error {
			return errors.Join(mcpPool.Shutdown(context.WithoutCancel(ctx)), deps.modelServices.callbacks.Close())
		})},
	}
	environment.admission = admission
	commandCalls, err := pluginservices.NewCommandCalls(cfg.stores.Installations, cfg.stores.ProviderSelections, cfg.stores.PluginCompositions, mcpPool, commands, deps.publish)
	if err != nil {
		return environment, err
	}
	environment.pluginCalls = commandCalls
	environment.callComposition, err = pluginservices.NewCallComposition(commandCalls, deps.modelServices.callbacks, admission)
	if err != nil {
		return environment, err
	}
	environment.modelCallbacks, err = environment.callComposition.ModelCallbacks(deps.modelServices.chatResolver)
	if err != nil {
		return environment, err
	}
	embedding, err := pluginservices.EmbeddingRoleCallback(deps.modelServices.liveEmbedder.Resolve)
	if err != nil {
		return environment, err
	}
	environment.modelCallbacks = append(environment.modelCallbacks, embedding)
	utility, err := modeladapter.LiveUtilityModel(deps.modelServices.chatResolver, deps.modelServices.defaultSelection, deps.modelServices.utilityRoleState, environment.callComposition)
	if err != nil {
		return environment, err
	}
	environment.utilityModel = environment.callComposition.Auxiliary(utility)
	environment.memory, err = pluginservices.NewMemory(pluginservices.MemoryConfig{Installations: cfg.stores.Installations, Connections: mcpPool, Callbacks: deps.modelServices.callbacks, Utility: environment.utilityModel, Embeddings: deps.modelServices.liveEmbedder.Resolve, Conversation: deps.conversation, Commands: commandCalls})
	if err != nil {
		return environment, err
	}
	environment.toolResults, err = pluginservices.NewToolResults(cfg.stores.Installations, mcpPool)
	if err != nil {
		return environment, err
	}
	buildConfig := toolset.BuildConfig{
		Lifetime:      deps.lifetime,
		UserHome:      cfg.userHome,
		SkillsUserDir: cfg.skillsUserDir, PackageSkills: deps.packageSkills,
		Online:          cfg.online,
		LSPServers:      cfg.lspServers,
		A2AAgents:       cfg.a2aAgents,
		Plan:            deps.plan,
		Interrupt:       runinput.Require,
		MCPToolDisabled: deps.mcp.exposure.ToolDisabled,
		SkillProposals:  deps.skillProposals,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.sandboxShell,
		SandboxReadOnlyPaths: cfg.sandboxReadOnlyPaths,
	}
	buildConfig.SkillUsage = deps.skillStore
	buildConfig.PlanMode = deps.approvalPolicy
	buildConfig.ToolResults = cfg.stores.ToolResults
	// create_goal is injected after Runs and the Driver exist.
	buildConfig.GoalReader = deps.goalReader
	buildConfig.GoalReporter = deps.goalReporter
	builtToolset, err := toolset.Build(ctx, buildConfig)
	if err != nil {
		return environment, fmt.Errorf("runtime: build tools: %w", err)
	}
	mcpPool.SetToolSink(builtToolset.Resolver.SetMCPTools)
	environment.tools = builtToolset
	environment.closers = append(environment.closers, terminalClosers(builtToolset.Closers)...)
	return environment, nil
}
