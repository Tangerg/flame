package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/recovery"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/application/automation/schedules"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
)

// Assembly owns configuration resources before construction begins.
type Assembly struct {
	mu         sync.Mutex
	cfg        Config
	buildTools toolEnvironmentBuilder
	lifetime   *hostLifetime
	started    bool
}

// NewAssembly acquires cfg.Resources and returns a single-use Host builder.
func NewAssembly(lifetime context.Context, cfg Config) *Assembly {
	return newAssembly(lifetime, cfg, buildToolEnvironment)
}

func newAssembly(
	lifetime context.Context,
	cfg Config,
	buildTools toolEnvironmentBuilder,
) *Assembly {
	return &Assembly{
		cfg:        cfg,
		buildTools: buildTools,
		lifetime: &hostLifetime{
			context:       lifetime,
			shutdownWait:  defaultShutdownWaitPolicy(),
			hostResources: terminalResources(cfg.Resources),
		},
	}
}

// BuildAssembly constructs and returns a complete Host. On failure it begins a
// bounded rollback and returns nil. The Host-owned shutdown generation keeps
// joining components and the terminal resource Sequence after caller timeout;
// CloseAssembly joins it or starts a new attempt after a settled component
// error.
func BuildAssembly(ctx context.Context, a *Assembly) (*Host, error) {
	if a == nil {
		return nil, errors.New("runtime: nil Assembly")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return nil, errors.New("runtime: BuildAssembly called more than once")
	}
	if a.lifetime == nil || a.buildTools == nil {
		return nil, errors.New("runtime: uninitialized Assembly")
	}
	if a.lifetime.context == nil {
		return nil, errors.New("runtime: Assembly lifetime is required")
	}
	a.started = true
	host, err := buildAssembly(ctx, a)
	if err != nil {
		if rollbackErr := closeHostLifetime(a.lifetime); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("runtime: rollback assembly: %w", rollbackErr))
		}
		return nil, err
	}
	a.lifetime = nil
	return host, nil
}

// CloseAssembly releases resources when BuildAssembly has not run, completes
// rollback after a failed build, and is a no-op after ownership transfers to a
// successful Host.
func CloseAssembly(a *Assembly) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Closing an unstarted Assembly consumes its single use. Otherwise a later
	// BuildAssembly could construct a Host over resources already released.
	a.started = true
	return closeHostLifetime(a.lifetime)
}

func buildAssembly(ctx context.Context, a *Assembly) (*Host, error) {
	if err := validateAssemblyConfig(a.cfg); err != nil {
		return nil, err
	}
	// Offloads are staged before their ordered transcript event commits so a
	// following model round can read them immediately. A process crash may leave
	// that short-lived stage behind; startup is the only point with no live tool
	// calls, so reconcile it before constructing the engine.
	if a.cfg.ToolResultStore != nil {
		if _, err := a.cfg.ToolResultStore.PurgeUnbound(ctx); err != nil {
			return nil, fmt.Errorf("runtime: reconcile staged tool results: %w", err)
		}
	}
	policy, err := buildPolicyComposition(ctx, a.cfg)
	if err != nil {
		return nil, err
	}
	workspaceServices, err := buildWorkspaceComposition(a.cfg, policy.invalidations.Publish)
	if err != nil {
		return nil, err
	}
	execution, err := buildExecutionComposition(
		ctx,
		a.cfg,
		a.lifetime,
		a.buildTools,
		policy,
		workspaceServices,
	)
	if err != nil {
		return nil, err
	}
	return buildAssemblyCore(ctx, a.cfg, a.lifetime, policy, workspaceServices, execution)
}

// buildAssemblyCore composes the Session/Run lifecycle from three complete
// feature capsules. No intermediate locator is published to Delivery.
func buildAssemblyCore(
	ctx context.Context,
	cfg Config,
	lifetime *hostLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
) (*Host, error) {
	fileChanges := newNotificationRelay[workspace.FileChangeNotice]()
	admissionGate, err := ownership.NewGate(cfg.SessionOwnership)
	if err != nil {
		return nil, fmt.Errorf("runtime: session admission: %w", err)
	}
	sessionStores := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		Interrupts:          cfg.InterruptStore,
		Runs:                cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		History:             execution.conversation.messages,
		Plan:                cfg.PlanStore,
		ApprovalRules:       cfg.ApprovalRuleStore,
		PermissionModes:     cfg.PermissionModeStore,
		ToolResults:         cfg.ToolResultStore,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Goals:               cfg.GoalStore,
		Tx:                  persistence.Transactor(cfg.Transactor),
	})
	modelCapabilities := modeladapter.Capabilities{}
	modelCoordinator := models.New(models.Config{
		Providers:          cfg.ProviderRegistry,
		Catalog:            modelCapabilities,
		Prober:             modelCapabilities,
		Lister:             modelCapabilities,
		UtilityRoleState:   execution.models.utilityRoleState,
		UtilityValidator:   execution.models.chatResolver,
		UtilityStore:       cfg.UtilityRoleStore,
		EmbeddingRoleState: execution.models.embeddingRoleState,
		EmbeddingValidator: execution.models.embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
		Invalidations:      policy.invalidations.Publish,
	})
	defaultRunModel, err := runtimeDefaultModelSelection(cfg)
	if err != nil {
		return nil, err
	}
	sessionDependencies := sessions.Dependencies{
		Sessions:              cfg.SessionStore,
		Interrupts:            cfg.InterruptStore,
		Transcript:            cfg.TranscriptStore,
		Runs:                  cfg.RunStore,
		Snapshots:             sessionStores,
		MaterialSnapshots:     sessionStores,
		Writes:                sessionStores,
		TransientState:        execution.transientSessions,
		ExecutionReleaser:     execution.executor,
		Paths:                 workspaceadapter.Resolver{},
		Models:                modelCapabilities,
		DefaultModelSelection: defaultRunModel,
		Checkpoints:           workspaceadapter.NewSessionCheckpoints(workspaceServices.checkpoints),
		Admissions:            admissionGate,
		Invalidations:         policy.invalidations.Publish,
		Now:                   time.Now,
		NewID:                 newSessionID,
		NewRunID:              newRunID,
		NewItemID:             newItemID,
		NewToolResultID:       toolresult.NewID,
	}
	if cfg.PlanStore != nil {
		sessionDependencies.Plan = &sessions.PlanServices{
			Boundaries: cfg.PlanStore, Replacements: policy.plans,
		}
	}
	if cfg.WorkspaceMutationStore != nil {
		sessionDependencies.Mutations = cfg.WorkspaceMutationStore
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if execution.isolation != nil {
		sessionDependencies.Sandbox = execution.isolation
	}
	// The shared Goal/session mutation coordinator is created before either
	// lifecycle owner. The Driver is constructed later because it consumes Runs;
	// no Bootstrap proxy or post-construction mutation is needed.
	var goalMutations *goals.SessionMutations
	if cfg.GoalStore != nil {
		goalMutations = goals.NewSessionMutations()
		sessionDependencies.Goals = goalMutations
	}
	sessionCoordinator, err := sessions.New(sessionDependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Session coordinator: %w", err)
	}
	// The Run coordinator owns the Run lifecycle (§20). Its driven persistence
	// adapter receives only Domain/Application-decided Session values; generated
	// title maintenance returns through the Session Application capability.
	runEffectTasks := &taskgroup.Group{}
	lifetime.runEffectTasks = runEffectTasks
	runSegmentConfig := segment.Config{
		Interrupts:          cfg.InterruptStore,
		ResumeClaims:        cfg.InterruptStore,
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		ItemReplacer:        cfg.TranscriptStore,
		ToolApprovals:       cfg.TranscriptStore,
		ModelInvocations:    cfg.ModelInvocationStore,
		ToolInvocations:     cfg.ToolInvocationStore,
		Conversation:        execution.conversation.store,
		State:               cfg.RunStore,
		RunProgress:         cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Tx:                  segment.Transactor(cfg.Transactor),
	}
	if cfg.ScheduleStore != nil {
		runSegmentConfig.Schedules = cfg.ScheduleStore
	}
	if cfg.GoalStore != nil {
		runSegmentConfig.GoalRuns = cfg.GoalStore
	}
	if cfg.ToolResultStore != nil {
		runSegmentConfig.ToolResults = cfg.ToolResultStore
	}
	runSegmentEffects, err := segment.New(runSegmentConfig)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run-segment effects: %w", err)
	}
	runFinalizer, err := segment.NewFinalizer(segment.FinalizerConfig{
		Checkpoints: workspaceServices.checkpoints,
		Titles: &segment.TitleMaintenance{
			Sessions:  sessionCoordinator,
			Generator: segment.NewTitleGenerator(execution.models.utilityClient),
			Tasks:     runEffectTasks,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run finalizer: %w", err)
	}
	workspaceNotifier := segment.NewWorkspaceNotifier(fileChanges.Publish)
	runDependencies := runs.Dependencies{
		RootStarts:                         execution.executor,
		Observations:                       execution.executor,
		Releases:                           execution.executor,
		RootCancellation:                   execution.executor,
		Conversation:                       execution.conversation.messages,
		Models:                             modelCapabilities,
		Continuation:                       execution.executor,
		WaitingRestorer:                    execution.executor,
		Steering:                           execution.executor,
		RunningSubtreeCanceler:             execution.executor,
		WaitingSubtreeCancellationPreparer: execution.executor,
		WorkingContexts:                    execution.workingContexts,
		Session: runs.SessionPorts{
			Reader:       sessionCoordinator,
			Creator:      sessionCoordinator,
			ActiveRuns:   sessionCoordinator,
			Interrupts:   sessionCoordinator,
			Terminations: sessionCoordinator,
		},
		Projection: runs.ProjectionPorts{
			Openings:                    runSegmentEffects,
			ChildStarts:                 runSegmentEffects,
			Checkpoints:                 runSegmentEffects,
			ResumeClaims:                runSegmentEffects,
			Events:                      runSegmentEffects,
			Barriers:                    runSegmentEffects,
			WaitingSubtreeCancellations: runSegmentEffects,
			Workspace:                   workspaceNotifier,
			Finalizer:                   runFinalizer,
		},
		Runs:          cfg.RunStore,
		Items:         cfg.TranscriptStore,
		Admissions:    admissionGate,
		Now:           time.Now,
		NewRunID:      newRunID,
		NewSegmentID:  newSegmentID,
		Invalidations: policy.invalidations.Publish,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if execution.isolation != nil {
		runDependencies.Isolation = execution.isolation
	}
	runCoordinator, err := runs.NewCoordinator(runDependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run coordinator: %w", err)
	}
	lifetime.runCoordinator = runCoordinator
	scheduleFiring := schedules.DisabledFiring()
	if cfg.ScheduleStore != nil {
		scheduleFiring, err = schedules.NewFiring(schedules.FiringDependencies{
			Store:         cfg.ScheduleStore,
			RunStarter:    schedules.NewRunLauncher(runCoordinator, cfg.DefaultWorkspacePath),
			NewSessionID:  newSessionID,
			NewRunID:      newRunID,
			Invalidations: policy.invalidations.Publish,
		})
		if err != nil {
			return nil, fmt.Errorf("runtime: construct Schedule firing: %w", err)
		}
	}

	approvalCoordinator := approvals.New(policy.approvals, cfg.SessionStore)

	toolCoordinator := workspace.NewDiagnosticTools(execution.toolRegistry, workspaceServices.scope)

	mcpCoordinator := mcpapp.New(mcpapp.Config{
		Registry:            cfg.MCPRegistry,
		StatusReader:        execution.tools.mcp,
		ToolCatalog:         execution.tools.mcp,
		ConnectionControl:   execution.tools.mcp,
		ConnectionLifecycle: execution.tools.mcp,
		Policy:              policy.mcp.policy,
		Invalidations:       policy.invalidations.Publish,
	})
	lifetime.mcpCoordinator = mcpCoordinator

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver, err = goals.NewDriver(
			policy.goals,
			runCoordinator,
			cfg.SessionStore,
			goalMutations,
			cfg.GoalDriveOwnership,
			builtin.RunInstructions,
		)
		if err != nil {
			return nil, fmt.Errorf("runtime: construct Goal driver: %w", err)
		}
		lifetime.goalDriver = goalDriver
		// create_goal is the only Goal tool that needs the Driver. Inject the
		// generic tool after Runs and the Driver exist. This must precede Run
		// recovery because it is part of the exact Deployment configuration used
		// to validate a durable executor checkpoint.
		createGoalTool, newCreateErr := builtin.NewCreate(goalDriver)
		if newCreateErr != nil {
			return nil, fmt.Errorf("runtime: build create_goal: %w", newCreateErr)
		}
		if execution.tools.tools.Resolver != nil {
			execution.tools.tools.Resolver.UseCreateGoalTool(createGoalTool)
		}
	}

	recoveryPersistence, err := recovery.New(recovery.Config{
		Sessions:            cfg.SessionStore,
		Runs:                cfg.RunStore,
		Interrupts:          cfg.InterruptStore,
		Transcript:          cfg.TranscriptStore,
		Messages:            execution.conversation.store,
		GoalRuns:            cfg.GoalStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		ModelInvocations:    cfg.ModelInvocationStore,
		ToolInvocations:     cfg.ToolInvocationStore,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Tx:                  recovery.Transactor(cfg.Transactor),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery persistence: %w", err)
	}
	bootRecovery, err := runs.NewRecovery(
		recoveryPersistence,
		execution.executor,
		admissionGate,
		policy.invalidations.Publish,
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery: %w", err)
	}
	var goalRecovery ownership.GoalRecovery
	if goalDriver != nil {
		goalRecovery = goalDriver
	}
	ownershipRecovery, err := ownership.NewRecovery(bootRecovery, goalRecovery, cfg.RecoveryOwnership)
	if err != nil {
		return nil, fmt.Errorf("runtime: ownership recovery: %w", err)
	}
	if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
		return nil, fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
	}
	workspaceFiles := workspace.NewFiles(workspaceServices.scope, workspaceadapter.FileBrowser{})
	workspaceVCS := workspace.NewVCS(workspaceServices.scope, workspaceadapter.VCS{})
	workspaceDiscovery := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	workspaceHooks := workspace.NewHooks(
		workspaceServices.scope, cfg.HooksResolver, cfg.HookTrustStore, policy.invalidations.Publish,
	)
	workspaceWatch := workspace.NewGitWatch(
		workspaceServices.scope,
		workspaceadapter.NewGitWatcher(lifetime.context),
	)
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		Transcript: cfg.TranscriptStore,
		Interrupts: cfg.InterruptStore,
		Runs:       cfg.RunStore,
		Sessions:   cfg.SessionStore,
		Plan:       cfg.PlanStore,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct session queries: %w", err)
	}
	usage, err := sessions.NewUsageReporter(sessions.UsageDependencies{
		Runs: cfg.RunStore, Sessions: cfg.SessionStore,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct usage reporter: %w", err)
	}
	feedback, err := sessions.NewFeedbackRecorder(cfg.FeedbackStore)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct feedback recorder: %w", err)
	}
	host := &Host{
		application: &hostApplication{
			delivery: delivery.HandlerConfig{
				Sessions:               sessionCoordinator,
				MCP:                    mcpCoordinator,
				Approvals:              approvalCoordinator,
				Models:                 modelCoordinator,
				Tools:                  toolCoordinator,
				Runs:                   runCoordinator,
				FileChanges:            fileChanges.Observe,
				Invalidations:          policy.invalidations.Observe,
				Queries:                queries,
				Usage:                  usage,
				Feedback:               feedback,
				WorkspaceFiles:         workspaceFiles,
				WorkspaceVCS:           workspaceVCS,
				WorkspaceDiscovery:     workspaceDiscovery,
				WorkspaceKnowledge:     workspaceServices.knowledge,
				WorkspaceSkills:        workspaceServices.skills,
				WorkspaceHooks:         workspaceHooks,
				WorkspaceWatch:         workspaceWatch,
				WorkspaceAuthoredWatch: workspaceServices.authoredWatch,
				Schedules:              policy.schedules,
				ScheduleFiring:         scheduleFiring,
				Goals:                  goalDriver,
				AgentMemory:            workspaceServices.agentMemory,
				GitAvailable:           workspaceadapter.GitAvailable(),
				PlanEnabled:            cfg.PlanStore != nil,
			},
			sessions: sessionCoordinator,
			workers: hostWorkers{
				scheduler:     scheduleFiring,
				recovery:      ownershipRecovery,
				invalidations: policy.invalidations.Publish,
			},
			idempotencyStore: cfg.IdempotencyStore,
		},
		lifetime: lifetime,
	}
	return host, nil
}
