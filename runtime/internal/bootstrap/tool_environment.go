package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools     toolset.Built
	mcp       *mcpconnection.Pool
	memory    *pluginservices.Memory
	schedules *pluginservices.Schedules
	closers   []*teardown.Step
	admission *plugins.Admission
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
	config         Config
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
	prepareTool, err := pluginservices.NewToolPreparation(pluginservices.ToolPreparationConfig{Installations: cfg.Stores.Installations, Callbacks: deps.modelServices.callbacks, Embeddings: deps.modelServices.liveEmbedder.ResolveMemory, Invalidations: deps.publish})
	if err != nil {
		return toolEnvironment{}, err
	}
	mcpPool, err := mcpconnection.Open(ctx, deps.lifetime, deps.mcp.servers, cfg.Stores.MCPAuthorization(), deps.registry, deps.modelServices.callbacks, prepareTool, deps.publish)
	if err != nil {
		return toolEnvironment{}, fmt.Errorf("runtime: open MCP connections: %w", err)
	}
	environment := toolEnvironment{
		mcp: mcpPool,
		// The SDK consumes each ClientSession transport closer even when Close
		// returns a diagnostic. Step owns the caller deadline; the action itself
		// must outlive that deadline and return only when the pool generation has
		// actually settled, so a timed-out Instance retains and later joins it.
		closers: []*teardown.Step{teardown.Terminal(func(ctx context.Context) error {
			return errors.Join(mcpPool.Shutdown(context.WithoutCancel(ctx)), deps.modelServices.callbacks.Close())
		})},
	}
	environment.admission = plugins.NewAdmission()
	commands, err := plugins.NewCommands(cfg.Stores.PluginCommands, environment.admission)
	if err != nil {
		return environment, err
	}
	commandCalls, err := pluginservices.NewCommandCalls(cfg.Stores.Installations, cfg.Stores.ProviderSelections, mcpPool, commands)
	if err != nil {
		return environment, err
	}
	environment.memory, err = pluginservices.NewMemory(pluginservices.MemoryConfig{Installations: cfg.Stores.Installations, Connections: mcpPool, Callbacks: deps.modelServices.callbacks, Utility: deps.modelServices.utilityModel, Embeddings: deps.modelServices.liveEmbedder.ResolveMemory, Conversation: deps.conversation, Invalidations: deps.publish, Commands: commandCalls})
	if err != nil {
		return environment, err
	}
	environment.schedules, err = pluginservices.NewSchedules(cfg.Stores.Installations, mcpPool, commandCalls, deps.publish)
	if err != nil {
		return environment, err
	}
	buildConfig := toolset.BuildConfig{
		Lifetime:      deps.lifetime,
		UserHome:      cfg.UserHome,
		SkillsUserDir: cfg.SkillsUserDir, PackageSkills: deps.packageSkills,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		A2AAgents:       cfg.A2AAgents,
		Plan:            deps.plan,
		Interrupt:       runinput.Require,
		MCPToolDisabled: deps.mcp.exposure.ToolDisabled,
		SkillProposals:  deps.skillProposals,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.SandboxShell,
		SandboxReadOnlyPaths: cfg.SandboxReadOnlyPaths,
	}
	buildConfig.SkillUsage = deps.skillStore
	buildConfig.PlanMode = deps.approvalPolicy
	buildConfig.ToolResults = cfg.Stores.ToolResults
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
