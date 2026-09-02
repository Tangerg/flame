package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/isolation"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/application/automation/schedules"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
)

const interactionDeploymentConfigurationIdentity = "flame.runtime.interaction.v1"

// policyComposition contains the application policies that share the same
// process-local invalidation vocabulary. It owns no background task or closer.
type policyComposition struct {
	invalidations notificationRelay[invalidation.Notice]
	approvals     *approvals.RuntimePolicy
	goals         goals.Store
	goalReader    *goals.Reader
	goalReporter  *goals.OutcomeReporter
	plans         *sessions.PlanCoordinator
	mcp           mcpEnvironment
	schedules     *schedules.Coordinator
}

func buildPolicyComposition(ctx context.Context, cfg Config) (policyComposition, error) {
	invalidations := newNotificationRelay[invalidation.Notice]()
	approvalPolicy, err := approvals.NewRuntimePolicy(
		cfg.ApprovalMode,
		cfg.ApprovalRuleStore,
		cfg.PermissionModeStore,
		invalidations.Publish,
	)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: approval policy: %w", err)
	}
	mcpSettings, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return policyComposition{}, err
	}
	goalStore := goals.WithInvalidations(cfg.GoalStore, invalidations.Publish)
	scheduleCoordinator := schedules.Disabled()
	if cfg.ScheduleStore != nil {
		scheduleCoordinator, err = schedules.New(schedules.Dependencies{
			Store:         cfg.ScheduleStore,
			Paths:         workspaceadapter.Resolver{},
			Models:        modeladapter.Capabilities{},
			NewScheduleID: newScheduleID,
			Invalidations: invalidations.Publish,
		})
		if err != nil {
			return policyComposition{}, fmt.Errorf("runtime: construct Schedule coordinator: %w", err)
		}
	}
	return policyComposition{
		invalidations: invalidations,
		approvals:     approvalPolicy,
		goals:         goalStore,
		goalReader:    goals.NewReader(goalStore),
		goalReporter:  goals.NewOutcomeReporter(goalStore),
		plans: sessions.NewPlanCoordinator(sessions.PlanDependencies{
			Store: cfg.PlanStore, Now: time.Now, Invalidations: invalidations.Publish,
		}),
		mcp:       mcpSettings,
		schedules: scheduleCoordinator,
	}, nil
}

// workspaceComposition is the complete authored-workspace capability. All
// members share one resolved scope and one authored-resource observer.
type workspaceComposition struct {
	scope            *workspace.Scope
	agentMemory      *agentmemoryapp.Coordinator
	memoryCuration   *agentmemoryapp.Curation
	authoredWatch    *workspace.AuthoredWatch
	knowledge        *workspace.Knowledge
	skills           *workspace.Skills
	skillMaintenance *workspace.SkillMaintenance
	skillStore       *skillauthoring.Store
	checkpoints      *workspaceadapter.Checkpoints
}

func buildWorkspaceComposition(
	cfg Config,
	publish invalidation.Publish,
) (workspaceComposition, error) {
	scope := workspace.NewScope(cfg.DefaultWorkspacePath, cfg.UserHome, workspaceadapter.Resolver{})
	authoredWatcher, err := workspaceadapter.NewAuthoredWatcher(
		cfg.KnowledgeDirectory,
		cfg.UserHome,
		cfg.SkillsUserDir,
	)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build authored resource watcher: %w", err)
	}
	authoredWatch := workspace.NewAuthoredWatch(scope, workspaceadapter.Resolver{}, authoredWatcher)
	knowledge := workspace.NewKnowledge(
		scope,
		workspaceadapter.Resolver{},
		cfg.KnowledgeStore,
		authoredWatch,
		publish,
	)
	skillStore := skillauthoring.NewStore(cfg.SkillsUserDir, skills.ScopeUser)
	var skillCurator workspace.SkillCurator
	var idleSkillSweeper workspace.IdleSkillSweeper
	if skillStore.Enabled() {
		skillCurator = skillStore
		idleSkillSweeper = skillStore
	}
	workspaceSkills := workspace.NewSkills(
		scope,
		promptsource.NewWorkspaceSkills(cfg.SkillsUserDir),
		skillCurator,
		workspaceadapter.NewSkillLibraries(skillStore),
		authoredWatch,
		publish,
	)
	return workspaceComposition{
		scope: scope,
		agentMemory: agentmemoryapp.New(agentmemoryapp.Config{
			Store: cfg.AgentMemoryStore, Roots: scope, Invalidations: publish,
		}),
		memoryCuration: agentmemoryapp.NewCuration(agentmemoryapp.CurationConfig{
			Store: cfg.AgentMemoryStore, Invalidations: publish,
		}),
		authoredWatch: authoredWatch,
		knowledge:     knowledge,
		skills:        workspaceSkills,
		skillMaintenance: workspace.NewSkillMaintenance(
			idleSkillSweeper,
			authoredWatch,
			publish,
		),
		skillStore:  skillStore,
		checkpoints: workspaceadapter.NewCheckpoints(cfg.CheckpointDir),
	}, nil
}

// executionComposition owns the model/tool execution graph. Every acquired
// closer is transferred to hostLifetime before an error can escape.
type executionComposition struct {
	conversation      conversationEnvironment
	models            modelEnvironment
	tools             toolEnvironment
	isolation         *isolation.Isolator
	workingContexts   *agentexec.WorkingContextComposer
	transientSessions *agentexec.TransientSessionState
	executor          *agentexec.InteractionExecutor
	toolRegistry      toolset.DiagnosticRegistry
}

func buildExecutionComposition(
	ctx context.Context,
	cfg Config,
	lifetime *hostLifetime,
	buildTools toolEnvironmentBuilder,
	policy policyComposition,
	workspaceServices workspaceComposition,
) (executionComposition, error) {
	conversation, err := buildConversationEnvironment(
		cfg.ConversationStore,
		persistence.NewConversationCompactions(
			cfg.ConversationStore,
			cfg.RunStore,
			persistence.Transactor(cfg.Transactor),
		),
	)
	if err != nil {
		return executionComposition{}, err
	}
	defaultSelection, err := runtimeDefaultModelSelection(cfg)
	if err != nil {
		return executionComposition{}, err
	}
	modelServices, err := buildModelEnvironment(ctx, cfg, defaultSelection)
	if err != nil {
		return executionComposition{}, err
	}
	// Isolated working copies are acquired before the shell set that can run in
	// them. The Host's reverse teardown therefore stops every detached shell
	// before it destroys the directories those processes use.
	var isolator *isolation.Isolator
	if cfg.SandboxDir != "" {
		isolator, err = isolation.New(cfg.UserHome, cfg.SandboxDir, cfg.SandboxReadOnlyPaths)
		if err != nil {
			return executionComposition{}, fmt.Errorf("runtime: build isolated workspace manager: %w", err)
		}
		lifetime.toolResources = append(lifetime.toolResources, teardown.Terminal(func(context.Context) error {
			return isolator.Close()
		}))
	}
	toolRuntime, err := buildTools(ctx, toolEnvironmentDependencies{
		lifetime:            lifetime.context,
		config:              cfg,
		approvalPolicy:      policy.approvals,
		mcp:                 policy.mcp,
		agentMemorySearcher: modelServices.agentMemorySearch,
		schedules:           policy.schedules,
		goalReader:          policy.goalReader,
		goalReporter:        policy.goalReporter,
		plan:                policy.plans,
		skillStore:          workspaceServices.skillStore,
		skillProposals:      workspaceServices.skills,
	})
	lifetime.toolResources = append(lifetime.toolResources, toolRuntime.closers...)
	if err != nil {
		return executionComposition{}, err
	}
	workingContexts := agentexec.NewWorkingContextComposer(agentexec.WorkingContextConfig{
		UserHome:          cfg.UserHome,
		Knowledge:         workspaceServices.knowledge,
		AgentMemory:       cfg.AgentMemoryStore,
		AgentMemorySearch: modelServices.agentMemorySearch,
		Plan:              cfg.PlanStore,
		Goal:              policy.goalReader,
		Hooks:             cfg.HooksResolver,
	})
	transientSessions := agentexec.NewTransientSessionState(
		workingContexts,
		toolRuntime.tools.Resolver,
		toolRuntime.tools.Shells,
	)
	toolAuthorizer, err := agentexec.NewToolAuthorizer(policy.approvals)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Tool authorizer: %w", err)
	}
	runMaintenance, modelContextCompactor, err := buildRunMaintenance(
		cfg,
		conversation,
		toolRuntime.tools.Shells,
		workspaceServices.skills,
		workspaceServices.skillMaintenance,
		workspaceServices.memoryCuration,
		modelServices.utilityClient,
		transientSessions,
	)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: build Run maintenance: %w", err)
	}
	maxConcurrentToolCalls := 8
	interactionConfig := agentexec.InteractionExecutorConfig{
		Lifetime:               lifetime.context,
		BuildID:                cfg.BuildID,
		ChatResolver:           cfg.ChatResolver,
		ImplementationIdentity: cfg.BuildID,
		ConfigurationIdentity:  interactionDeploymentConfigurationIdentity,
		StreamModelResponses:   true,
		MaxConcurrentToolCalls: &maxConcurrentToolCalls,
		ToolInterpreter:        toolset.NewInterpreter(policy.plans),
		ToolPresenter:          toolset.Presenter{},
		ToolAuthorizer:         toolAuthorizer,
		ToolHooks:              workingContexts,
		MCPToolAutoApproved: func(server, toolName string) bool {
			name, err := mcpserver.ParseServerName(server)
			if err != nil {
				return false
			}
			remoteName, err := mcpserver.ParseRemoteToolName(toolName)
			if err != nil {
				return false
			}
			return policy.mcp.policy.ToolAutoApproved(mcpserver.ToolRef{Server: name, Tool: remoteName})
		},
		Maintenance:           runMaintenance,
		ModelContextCompactor: modelContextCompactor,
		ModelContextState:     workingContexts,
		LifecycleHooks:        workingContexts,
		Pricing:               cfg.Pricing,
	}
	if toolRuntime.tools.Resolver != nil {
		interactionConfig.ToolResolver = toolRuntime.tools.Resolver
	}
	if cfg.ToolResultOffloadEnabled {
		toolResultThreshold := cfg.ToolResultThreshold
		interactionConfig.ToolResultStore = cfg.ToolResultStore
		interactionConfig.ToolResultOffload = agentexec.ToolResultOffloadPolicyValues{
			Threshold:  &toolResultThreshold,
			ReaderName: tool.ReadToolResult,
		}
	}
	interactionExecutor, err := agentexec.NewInteractionExecutor(interactionConfig)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Interaction executor: %w", err)
	}
	lifetime.executor = interactionExecutor
	return executionComposition{
		conversation:      conversation,
		models:            modelServices,
		tools:             toolRuntime,
		isolation:         isolator,
		workingContexts:   workingContexts,
		transientSessions: transientSessions,
		executor:          interactionExecutor,
		toolRegistry:      toolset.NewDiagnosticRegistry(),
	}, nil
}

func runtimeDefaultModelSelection(cfg Config) (modelref.Selection, error) {
	selection, err := modelref.New(cfg.Provider, cfg.Model)
	if err != nil {
		return modelref.Selection{}, fmt.Errorf("runtime: default model selection: %w", err)
	}
	if !selection.Configured() {
		return modelref.Selection{}, errors.New("runtime: configured default model selection is required")
	}
	return selection, nil
}
