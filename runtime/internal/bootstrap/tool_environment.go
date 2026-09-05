package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/application/automation/schedules"
	"github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
)

// toolEnvironment groups the tool resolver with the separately-owned MCP
// connection adapter. Bootstrap is the composition root that joins them; the
// generic toolset does not expose application integration ports.
type toolEnvironment struct {
	tools   toolset.Built
	mcp     *mcpconnection.Pool
	closers []*teardown.Step
}

// toolEnvironmentDependencies is the complete construction contract for the
// process-owned tool runtime. Keeping the contract as one value lets construction
// transfer every acquisition to runtimeLifetime even when construction fails.
type toolEnvironmentDependencies struct {
	lifetime          context.Context
	config            Config
	approvalPolicy    *approvals.RuntimePolicy
	mcp               mcpEnvironment
	agentMemoryReader *agentmemory.ReadModel
	schedules         *schedules.Coordinator
	goalReader        *goals.Reader
	goalReporter      *goals.OutcomeReporter
	plan              *sessions.PlanCoordinator
	skillStore        *skillauthoring.Store
	skillProposals    builtin.SkillProposalSubmitter
}

type toolEnvironmentBuilder func(context.Context, toolEnvironmentDependencies) (toolEnvironment, error)

func buildToolEnvironment(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
	cfg := deps.config
	mcpPool, mcpTools, err := mcpconnection.Open(
		ctx,
		deps.lifetime,
		deps.mcp.servers,
		cfg.Stores.MCPServers,
	)
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
			return mcpPool.Shutdown(context.WithoutCancel(ctx))
		})},
	}
	buildConfig := toolset.BuildConfig{
		Lifetime:        deps.lifetime,
		DefaultCWD:      cfg.DefaultWorkspacePath,
		UserHome:        cfg.UserHome,
		SkillsUserDir:   cfg.SkillsUserDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPTools:        mcpTools,
		A2AAgents:       cfg.A2AAgents,
		Plan:            deps.plan,
		Interrupt:       agentexec.RequireToolInput,
		MCPToolDisabled: deps.mcp.policy.ToolDisabled,
		// The authoring store records Skill loads for idle-Skill archival; a
		// disabled store no-ops RecordUse.
		SkillUsage:     deps.skillStore,
		SkillProposals: deps.skillProposals,
		// Opt-in per-command OS isolation for the shell tools (off by default).
		SandboxShell:         cfg.SandboxShell,
		SandboxReadOnlyPaths: cfg.SandboxReadOnlyPaths,
	}
	buildConfig.PlanMode = deps.approvalPolicy
	buildConfig.Schedules = deps.schedules
	buildConfig.ToolResults = cfg.Stores.ToolResults
	// create_goal is injected after Runs and the Driver exist.
	buildConfig.GoalReader = deps.goalReader
	buildConfig.GoalReporter = deps.goalReporter
	buildConfig.AgentMemorySearch = deps.agentMemoryReader
	buildConfig.ConversationSearch = cfg.Stores.Transcript
	builtToolset, err := toolset.Build(ctx, buildConfig)
	if err != nil {
		return environment, fmt.Errorf("runtime: build tools: %w", err)
	}
	mcpPool.SetToolSink(builtToolset.Resolver.SetMCPTools)
	environment.tools = builtToolset
	environment.closers = append(environment.closers, terminalClosers(builtToolset.Closers)...)
	return environment, nil
}
