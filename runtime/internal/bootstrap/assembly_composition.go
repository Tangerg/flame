package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

// policyComposition contains the application policies that share the same
// process-local invalidation vocabulary. It owns no background task or closer.
type policyComposition struct {
	packages      *pluginpackage.Releases
	packageSkills *pluginpackage.Skills
	registry      *plugins.Registry
	invalidations notificationRelay[invalidation.Notice]
	approvals     *approvals.RuntimePolicy
	goals         goals.Store
	goalReader    *goals.Reader
	goalReporter  *goals.OutcomeReporter
	plans         *sessions.PlanCoordinator
	mcp           mcpEnvironment
}

func buildPolicyComposition(ctx context.Context, cfg assemblyConfig) (policyComposition, error) {
	invalidations := newNotificationRelay[invalidation.Notice]()
	packages, err := pluginpackage.New(filepath.Join(cfg.stores.DataDirectory, "plugins", "releases"), cfg.stores.PluginReleases)
	if err != nil {
		return policyComposition{}, err
	}
	packageSkills, err := pluginpackage.NewSkills(packages, cfg.stores.Installations)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: construct package skills: %w", err)
	}
	registry, err := plugins.NewRegistry(cfg.stores.MCPServers, cfg.stores.Installations, packages, cfg.stores.ProviderSelections, cfg.stores.PluginCompositions)
	if err != nil {
		return policyComposition{}, err
	}
	approvalPolicy, err := approvals.NewRuntimePolicy(
		cfg.approvalMode,
		cfg.stores.ApprovalRules,
		cfg.stores.ApprovalModes,
		toolset.NewAuthorities(registry.Definition, cfg.a2aAgents),
		invalidations.publish,
	)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: approval policy: %w", err)
	}
	mcpSettings, err := buildMCPEnvironment(ctx, registry, cfg.stores.MCPServers)
	if err != nil {
		return policyComposition{}, err
	}
	goalStore := goals.WithInvalidations(cfg.stores.Goals, invalidations.publish)
	goalReader, err := goals.NewReader(goalStore)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: build goal reader: %w", err)
	}
	goalReporter, err := goals.NewOutcomeReporter(goalStore)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: build goal outcome reporter: %w", err)
	}
	plans, err := sessions.NewPlanCoordinator(sessions.PlanDependencies{
		Store: cfg.stores.Plan, Now: time.Now, Invalidations: invalidations.publish,
	})
	if err != nil {
		return policyComposition{}, err
	}
	return policyComposition{
		invalidations: invalidations, packages: packages, registry: registry,
		packageSkills: packageSkills,
		approvals:     approvalPolicy,
		goals:         goalStore,
		goalReader:    goalReader,
		goalReporter:  goalReporter,
		plans:         plans,
		mcp:           mcpSettings,
	}, nil
}

// workspaceComposition is the complete authored-workspace capability. All
// members share one resolved scope and one authored-resource observer.
type workspaceComposition struct {
	scope            *workspace.Scope
	hookResolver     *adapterhooks.Resolver
	hooks            *apphooks.Catalog
	authoredWatch    *workspace.AuthoredWatch
	skills           *workspace.Skills
	skillMaintenance *workspace.SkillMaintenance
	skillStore       *skillauthoring.Store
	checkpoints      *workspaceadapter.Checkpoints
}

func buildWorkspaceComposition(
	cfg assemblyConfig,
	publish invalidation.Publish,
	packageSkills promptsource.PackageSkills,
) (workspaceComposition, error) {
	scope, err := workspace.NewScope(cfg.defaultWorkspacePath, cfg.userHome, workspaceadapter.Resolver{})
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build workspace scope: %w", err)
	}
	hookResolver, err := newHookResolver(cfg.userHome, cfg.stores.Trust)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build hook resolver: %w", err)
	}
	hooks, err := apphooks.NewCatalog(scope, hookResolver, cfg.stores.Trust, publish)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build hook management: %w", err)
	}
	authoredWatcher, err := workspaceadapter.NewAuthoredWatcher(cfg.userHome, cfg.skillsUserDir)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build authored resource watcher: %w", err)
	}
	authoredWatch, err := workspace.NewAuthoredWatch(scope, workspaceadapter.Resolver{}, authoredWatcher)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build authored resource observation: %w", err)
	}
	skillStore, err := skillauthoring.NewStore(cfg.skillsUserDir, skills.ScopeUser)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build user skill store: %w", err)
	}
	skillMaintenance, err := workspace.NewSkillMaintenance(skillStore, authoredWatch, publish)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build skill maintenance: %w", err)
	}
	skillLibraries, err := workspaceadapter.NewSkillLibraries(skillStore)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build skill libraries: %w", err)
	}
	workspaceSkills, err := workspace.NewSkills(
		scope,
		promptsource.NewSkills(cfg.skillsUserDir, packageSkills),
		skillStore,
		skillLibraries,
		authoredWatch,
		publish,
	)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build skills: %w", err)
	}
	return workspaceComposition{
		scope:            scope,
		hookResolver:     hookResolver,
		hooks:            hooks,
		authoredWatch:    authoredWatch,
		skills:           workspaceSkills,
		skillMaintenance: skillMaintenance,
		skillStore:       skillStore,
		checkpoints:      workspaceadapter.NewCheckpoints(cfg.checkpointDir),
	}, nil
}

// executionComposition owns the model/tool execution graph. Every acquired
// closer is transferred to runtimeLifetime before an error can escape.
type executionComposition struct {
	conversation      conversationEnvironment
	models            modelEnvironment
	tools             toolEnvironment
	isolation         *workspaceadapter.Isolator
	workingContexts   *executionadapter.WorkingContextComposer
	transientSessions *executionadapter.TransientSessionState
	executor          *executionadapter.InteractionExecutor
	toolRegistry      toolset.DiagnosticRegistry
}

func buildExecutionComposition(
	ctx context.Context,
	cfg assemblyConfig,
	defaultSelection modelchoice.Selection,
	lifetime *runtimeLifetime,
	buildTools toolEnvironmentBuilder,
	policy policyComposition,
	workspaceServices workspaceComposition,
) (executionComposition, error) {
	conversationStore, err := persistence.NewConversationStore(cfg.stores.ChatHistory)
	if err != nil {
		return executionComposition{}, err
	}
	compactions, err := persistence.NewConversationCompactions(
		conversationStore,
		cfg.stores.Runs,
		persistence.Transactor(cfg.stores.Transactor),
	)
	if err != nil {
		return executionComposition{}, err
	}
	conversation, err := buildConversationEnvironment(conversationStore, compactions)
	if err != nil {
		return executionComposition{}, err
	}
	modelServices, err := buildModelEnvironment(ctx, cfg, defaultSelection)
	if err != nil {
		return executionComposition{}, err
	}
	// Isolated working copies are acquired before the shell set that can run in
	// them. The Instance's reverse teardown therefore stops every detached shell
	// before it destroys the directories those processes use.
	var isolator *workspaceadapter.Isolator
	if cfg.sandboxDir != "" {
		isolator, err = workspaceadapter.NewIsolator(cfg.userHome, cfg.sandboxDir, cfg.sandboxReadOnlyPaths)
		if err != nil {
			return executionComposition{}, fmt.Errorf("runtime: build isolated workspace manager: %w", err)
		}
		lifetime.toolResources = append(lifetime.toolResources, terminalStep(func(context.Context) error {
			return isolator.Close()
		}))
	}
	toolRuntime, err := buildTools(ctx, toolEnvironmentDependencies{
		registry: policy.registry, packageSkills: policy.packageSkills,
		lifetime:       lifetime.context,
		config:         cfg,
		approvalPolicy: policy.approvals,
		mcp:            policy.mcp,
		modelServices:  modelServices,
		workspaceScope: workspaceServices.scope,
		conversation:   conversation.store,
		publish:        policy.invalidations.publish,
		goalReader:     policy.goalReader,
		goalReporter:   policy.goalReporter,
		plan:           policy.plans,
		skillStore:     workspaceServices.skillStore,
		skillProposals: workspaceServices.skills,
	})
	lifetime.toolResources = append(lifetime.toolResources, toolRuntime.closers...)
	if err != nil {
		return executionComposition{}, err
	}
	modelServices.utilityModel = toolRuntime.utilityModel
	workingContexts, err := executionadapter.NewWorkingContextComposer(executionadapter.WorkingContextConfig{
		UserHome:          cfg.userHome,
		AgentMemory:       toolRuntime.memory,
		AgentMemorySearch: toolRuntime.memory,
		Plan:              cfg.stores.Plan,
		Goal:              policy.goalReader,
		Hooks:             workspaceServices.hookResolver,
	})
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: build working context: %w", err)
	}
	transientSessions, err := executionadapter.NewTransientSessionState(
		workingContexts,
		toolRuntime.tools.Resolver,
		toolRuntime.tools.Shells,
	)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: build transient Session state: %w", err)
	}
	toolInterpreter := toolset.NewInterpreter(policy.plans)
	toolAuthorizer, err := executionadapter.NewToolAuthorizer(policy.approvals, toolInterpreter)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Tool authorizer: %w", err)
	}
	runMaintenance, modelContextCompactor, err := buildRunMaintenance(
		cfg,
		conversation,
		toolRuntime.tools.Shells,
		workspaceServices.skills,
		workspaceServices.skillMaintenance,
		toolRuntime.memory,
		modelServices.utilityModel,
	)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: build Run maintenance: %w", err)
	}
	maxConcurrentToolCalls := 8
	interactionConfig := executionadapter.InteractionExecutorConfig{
		Lifetime:                lifetime.context,
		ExecutionTrees:          cfg.stores.ExecutorCheckpoints,
		OriginalToolResults:     toolRuntime.toolResults,
		InstallationCheckpoints: cfg.stores.ExecutorCheckpoints,
		Admission:               toolRuntime.admission,
		InstallationCommands:    cfg.stores.PluginCommands,
		BuildID:                 cfg.buildID,
		ChatResolver:            modelServices.chatResolver,
		ModelEvidence:           cfg.stores.ModelInvocations,
		CallComposition:         toolRuntime.callComposition,
		StreamModelResponses:    true,
		MaxConcurrentToolCalls:  &maxConcurrentToolCalls,
		ToolInterpreter:         toolInterpreter,
		ToolPresenter:           toolset.Presenter{},
		ToolAuthorizer:          toolAuthorizer,
		ToolHooks:               workingContexts,
		Maintenance:             runMaintenance,
		ModelContextCompactor:   modelContextCompactor,
		ModelContextState:       workingContexts,
		LifecycleHooks:          workingContexts,
	}
	if toolRuntime.tools.Resolver != nil {
		interactionConfig.ToolResolver = toolRuntime.tools.Resolver
	}
	if cfg.toolResultOffloadEnabled {
		toolResultThreshold := cfg.toolResultThreshold
		interactionConfig.ToolResultStore = cfg.stores.ToolResults
		interactionConfig.ToolResultOffload = executionadapter.ToolResultOffloadPolicyValues{
			Threshold:  &toolResultThreshold,
			ReaderName: string(tool.ReadToolResult),
		}
	}
	interactionExecutor, err := executionadapter.NewInteractionExecutor(interactionConfig)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Interaction executor: %w", err)
	}
	lifetime.executor = interactionExecutor
	toolRegistry, err := toolset.NewDiagnosticRegistry(cfg.defaultWorkspacePath)
	if err != nil {
		return executionComposition{}, err
	}
	return executionComposition{
		conversation:      conversation,
		models:            modelServices,
		tools:             toolRuntime,
		isolation:         isolator,
		workingContexts:   workingContexts,
		transientSessions: transientSessions,
		executor:          interactionExecutor,
		toolRegistry:      toolRegistry,
	}, nil
}

func runtimeDefaultModelSelection(cfg assemblyConfig) (modelchoice.Selection, error) {
	selection, err := modelchoice.New(cfg.provider, cfg.model)
	if err == nil {
		err = selection.ValidateExact()
	}
	if err != nil {
		return modelchoice.Selection{}, fmt.Errorf("runtime: default model selection: %w", err)
	}
	return selection, nil
}
