package bootstrap

import (
	"context"
	"errors"
	"fmt"
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

// assemble transfers acquired resources directly to the Runtime lifecycle.
// Construction state stays on this stack; rollback uses the same shutdown owner
// as a successfully opened Runtime and survives a caller's timeout.
func assemble(ctx context.Context, cfg Config, lifetime *runtimeLifetime, buildTools toolEnvironmentBuilder) (_ *Instance, err error) {
	defer func() {
		if err != nil {
			if rollbackErr := closeRuntimeLifetime(lifetime); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("runtime: rollback assembly: %w", rollbackErr))
			}
		}
	}()
	if err := validateAssemblyConfig(cfg); err != nil {
		return nil, err
	}
	defaultSelection, err := runtimeDefaultModelSelection(cfg)
	if err != nil {
		return nil, err
	}
	// Offloads are staged before their ordered transcript event commits so a
	// following model round can read them immediately. A process crash may leave
	// that short-lived stage behind; startup is the only point with no live tool
	// calls, so reconcile it before constructing the engine.
	if _, err := cfg.Stores.ToolResults.PurgeUnbound(ctx); err != nil {
		return nil, fmt.Errorf("runtime: reconcile staged tool results: %w", err)
	}
	policy, err := buildPolicyComposition(ctx, cfg)
	if err != nil {
		return nil, err
	}
	workspaceServices, err := buildWorkspaceComposition(cfg, policy.invalidations.Publish)
	if err != nil {
		return nil, err
	}
	execution, err := buildExecutionComposition(
		ctx,
		cfg,
		defaultSelection,
		lifetime,
		buildTools,
		policy,
		workspaceServices,
	)
	if err != nil {
		return nil, err
	}
	return buildAssemblyCore(ctx, cfg, lifetime, policy, workspaceServices, execution)
}

// buildAssemblyCore composes the Session/Run lifecycle from three complete
// feature capsules. No intermediate locator is published to Delivery.
func buildAssemblyCore(
	ctx context.Context,
	cfg Config,
	lifetime *runtimeLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
) (*Instance, error) {
	fileChanges := newNotificationRelay[workspace.FileChangeNotice]()
	admissionGate, err := ownership.NewGate(cfg.SessionOwnership)
	if err != nil {
		return nil, fmt.Errorf("runtime: session admission: %w", err)
	}
	sessionStores := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.Stores.Sessions,
		Transcript:          cfg.Stores.Transcript,
		Interrupts:          cfg.Stores.Interrupts,
		Runs:                cfg.Stores.Runs,
		ExecutorCheckpoints: cfg.Stores.ExecutorCheckpoints,
		History:             execution.conversation.messages,
		Plan:                cfg.Stores.Plan,
		ApprovalRules:       cfg.Stores.ApprovalRules,
		PermissionModes:     cfg.Stores.PermissionModes,
		ToolResults:         cfg.Stores.ToolResults,
		ChildRunStarts:      cfg.Stores.ChildRunStarts,
		Goals:               cfg.Stores.Goals,
		Tx:                  persistence.Transactor(cfg.Stores.Transactor),
	})
	modelCapabilities := modeladapter.Capabilities{}
	modelCoordinator, err := models.New(models.Config{
		Providers:          cfg.ProviderRegistry,
		Catalog:            modelCapabilities,
		Prober:             modelCapabilities,
		Lister:             modelCapabilities,
		UtilityRoleState:   execution.models.utilityRoleState,
		UtilityValidator:   execution.models.chatResolver,
		UtilityStore:       cfg.Stores.UtilityRole,
		EmbeddingRoleState: execution.models.embeddingRoleState,
		EmbeddingValidator: execution.models.embeddingResolver,
		EmbeddingStore:     cfg.Stores.EmbeddingRole,
		Invalidations:      policy.invalidations.Publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct model coordinator: %w", err)
	}
	sessionDependencies := sessions.Dependencies{
		Sessions:              cfg.Stores.Sessions,
		Interrupts:            cfg.Stores.Interrupts,
		Transcript:            cfg.Stores.Transcript,
		Runs:                  cfg.Stores.Runs,
		Snapshots:             sessionStores,
		MaterialSnapshots:     sessionStores,
		Writes:                sessionStores,
		TransientState:        execution.transientSessions,
		ExecutionReleaser:     execution.executor,
		Paths:                 workspaceadapter.Resolver{},
		Models:                modelCapabilities,
		DefaultModelSelection: execution.models.defaultSelection,
		Checkpoints:           workspaceadapter.NewSessionCheckpoints(workspaceServices.checkpoints),
		Admissions:            admissionGate,
		Invalidations:         policy.invalidations.Publish,
		Now:                   time.Now,
		NewID:                 newSessionID,
		NewRunID:              newRunID,
		NewItemID:             newItemID,
		NewToolResultID:       toolresult.NewID,
	}
	sessionDependencies.Plan = sessions.PlanServices{
		Boundaries: cfg.Stores.Plan, Replacements: policy.plans,
	}
	sessionDependencies.Mutations = cfg.Stores.WorkspaceMutations
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if execution.isolation != nil {
		sessionDependencies.Sandbox = execution.isolation
	}
	// The shared Goal/session mutation coordinator is created before either
	// lifecycle owner. The Driver is constructed later because it consumes Runs;
	// no Bootstrap proxy or post-construction mutation is needed.
	goalMutations := goals.NewSessionMutations()
	sessionDependencies.Goals = goalMutations
	sessionCoordinator, err := sessions.New(sessionDependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Session coordinator: %w", err)
	}
	// The Run coordinator owns the Run lifecycle. Its driven persistence
	// adapter receives only Domain/Application-decided Session values; generated
	// title maintenance returns through the Session Application capability.
	runEffectTasks := &taskgroup.Group{}
	lifetime.runEffectTasks = runEffectTasks
	runSegmentConfig := segment.Config{
		Interrupts:          cfg.Stores.Interrupts,
		ResumeClaims:        cfg.Stores.Interrupts,
		Sessions:            cfg.Stores.Sessions,
		Transcript:          cfg.Stores.Transcript,
		ItemReplacer:        cfg.Stores.Transcript,
		ToolApprovals:       cfg.Stores.Transcript,
		ModelInvocations:    cfg.Stores.ModelInvocations,
		ToolInvocations:     cfg.Stores.ToolInvocations,
		Conversation:        execution.conversation.store,
		State:               cfg.Stores.Runs,
		RunProgress:         cfg.Stores.Runs,
		ExecutorCheckpoints: cfg.Stores.ExecutorCheckpoints,
		ChildRunStarts:      cfg.Stores.ChildRunStarts,
		Tx:                  segment.Transactor(cfg.Stores.Transactor),
	}
	runSegmentConfig.Schedules = cfg.Stores.Schedules
	runSegmentConfig.GoalRuns = cfg.Stores.Goals
	runSegmentConfig.ToolResults = cfg.Stores.ToolResults
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
		Runs:          cfg.Stores.Runs,
		Items:         cfg.Stores.Transcript,
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
	scheduleFiring, err := schedules.NewFiring(schedules.FiringDependencies{
		Store:         cfg.Stores.Schedules,
		RunStarter:    schedules.NewRunLauncher(runCoordinator, cfg.DefaultWorkspacePath),
		NewSessionID:  newSessionID,
		NewRunID:      newRunID,
		Invalidations: policy.invalidations.Publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Schedule firing: %w", err)
	}

	approvalCoordinator := approvals.New(policy.approvals, sessionCoordinator)

	toolCoordinator := workspace.NewDiagnosticTools(execution.toolRegistry, workspaceServices.scope)

	mcpCoordinator, err := mcpapp.New(mcpapp.Config{
		Registry:            cfg.Stores.MCPServers,
		StatusReader:        execution.tools.mcp,
		ToolCatalog:         execution.tools.mcp,
		ConnectionControl:   execution.tools.mcp,
		ConnectionLifecycle: execution.tools.mcp,
		Policy:              policy.mcp.policy,
		Invalidations:       policy.invalidations.Publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct MCP coordinator: %w", err)
	}
	lifetime.mcpCoordinator = mcpCoordinator

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	goalDriver, err := goals.NewDriver(
		policy.goals,
		runCoordinator,
		cfg.Stores.Sessions,
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

	recoveryPersistence, err := recovery.New(recovery.Config{
		Sessions:            cfg.Stores.Sessions,
		Runs:                cfg.Stores.Runs,
		Interrupts:          cfg.Stores.Interrupts,
		Transcript:          cfg.Stores.Transcript,
		Messages:            execution.conversation.store,
		GoalRuns:            cfg.Stores.Goals,
		ExecutorCheckpoints: cfg.Stores.ExecutorCheckpoints,
		ModelInvocations:    cfg.Stores.ModelInvocations,
		ToolInvocations:     cfg.Stores.ToolInvocations,
		ChildRunStarts:      cfg.Stores.ChildRunStarts,
		Tx:                  recovery.Transactor(cfg.Stores.Transactor),
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

	ownershipRecovery, err := ownership.NewRecovery(bootRecovery, goalDriver, cfg.RecoveryOwnership)
	if err != nil {
		return nil, fmt.Errorf("runtime: ownership recovery: %w", err)
	}
	if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
		return nil, fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
	}
	workspaceFiles, err := workspace.NewFiles(workspaceServices.scope, workspaceadapter.FileBrowser{})
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace files: %w", err)
	}
	workspaceVCS := workspace.NewVCS(workspaceServices.scope, workspaceadapter.VCS{})
	workspaceDiscovery, err := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace discovery: %w", err)
	}
	workspaceWatch := workspace.NewGitWatch(
		workspaceServices.scope,
		workspaceadapter.NewGitWatcher(lifetime.context),
	)
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		Transcript: cfg.Stores.Transcript,
		Interrupts: cfg.Stores.Interrupts,
		Runs:       cfg.Stores.Runs,
		Sessions:   cfg.Stores.Sessions,
		Plan:       cfg.Stores.Plan,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct session queries: %w", err)
	}
	usage, err := sessions.NewUsageReporter(sessions.UsageDependencies{
		Runs: cfg.Stores.Runs, Sessions: sessionCoordinator,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct usage reporter: %w", err)
	}
	feedback, err := sessions.NewFeedbackRecorder(cfg.Stores.Feedback)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct feedback recorder: %w", err)
	}
	host := &Instance{
		application: &runtimeApplication{
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
				WorkspaceHooks:         workspaceServices.hooks,
				WorkspaceWatch:         workspaceWatch,
				WorkspaceAuthoredWatch: workspaceServices.authoredWatch,
				Schedules:              policy.schedules,
				ScheduleFiring:         scheduleFiring,
				Goals:                  goalDriver,
				AgentMemory:            workspaceServices.agentMemory,
				GitAvailable:           workspaceadapter.GitAvailable(),
			},
			sessions: sessionCoordinator,
			workers: runtimeWorkers{
				scheduler:     scheduleFiring,
				recovery:      ownershipRecovery,
				invalidations: policy.invalidations.Publish,
			},
			idempotencyStore: cfg.Stores.Idempotency,
		},
		lifetime: lifetime,
	}
	return host, nil
}
