package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/recovery"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/feedback"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

// assemble transfers acquired resources directly to the Runtime lifecycle.
// Construction state stays on this stack; rollback uses the same shutdown owner
// as a successfully opened Runtime and survives a caller's timeout.
func assemble(ctx context.Context, cfg assemblyConfig, lifetime *runtimeLifetime, buildTools toolEnvironmentBuilder) (_ *Instance, err error) {
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
	callbacks := qimumcp.NewCallbacks()
	lifetime.toolResources = append(lifetime.toolResources, terminalStep(func(context.Context) error { return callbacks.Close() }))
	policy, err := buildPolicyComposition(ctx, cfg, lifetime, callbacks)
	if err != nil {
		return nil, err
	}
	workspaceServices, err := buildWorkspaceComposition(cfg, policy.invalidations.publish, policy.packageSkills)
	if err != nil {
		return nil, err
	}
	execution, err := buildExecutionComposition(
		ctx,
		cfg,
		defaultSelection,
		callbacks,
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
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
) (*Instance, error) {
	fileChanges := newNotificationRelay[workspace.FileChangeNotice]()
	goalRuns, err := goals.NewRunRecorder(cfg.stores.Goals)
	if err != nil {
		return nil, fmt.Errorf("runtime: Goal Run recorder: %w", err)
	}
	admissionGate, err := ownership.NewGate(filesystem.CanonicalPath)
	if err != nil {
		return nil, fmt.Errorf("runtime: session admission: %w", err)
	}
	sessionStores, err := persistence.NewSessionStores(persistence.SessionStoresConfig{
		CommandAcceptances:  cfg.stores.CommandAcceptances,
		Sessions:            cfg.stores.Sessions,
		Transcript:          cfg.stores.Transcript,
		Interrupts:          cfg.stores.Interrupts,
		Runs:                cfg.stores.Runs,
		ExecutorCheckpoints: cfg.stores.ExecutorCheckpoints,
		History:             execution.conversation.messages,
		Plan:                cfg.stores.Plan,
		ApprovalRules:       cfg.stores.ApprovalRules,
		ApprovalModes:       cfg.stores.ApprovalModes,
		ToolResults:         cfg.stores.ToolResults,
		ChildRunStarts:      cfg.stores.ChildRunStarts,
		Goals:               cfg.stores.Goals,
		GoalRuns:            goalRuns,
		Tx:                  persistence.Transactor(cfg.stores.Transactor),
		ReadSnapshot:        persistence.Transactor(cfg.stores.ReadSnapshot),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: session write-set stores: %w", err)
	}
	modelCapabilities := execution.models.capabilities
	modelCoordinator, err := models.New(models.Config{
		Providers:          cfg.providerRegistry,
		Catalog:            modelCapabilities,
		Prober:             modelCapabilities,
		Lister:             modelCapabilities,
		UtilityRoleState:   execution.models.utilityRoleState,
		UtilityValidator:   execution.models.chatResolver,
		UtilityStore:       cfg.stores.UtilityRole,
		EmbeddingRoleState: execution.models.embeddingRoleState,
		EmbeddingValidator: execution.models.embeddingResolver,
		EmbeddingStore:     cfg.stores.EmbeddingRole,
		Invalidations:      policy.invalidations.publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct model coordinator: %w", err)
	}
	sessionDependencies := sessions.Dependencies{
		Sessions:              cfg.stores.Sessions,
		Interrupts:            cfg.stores.Interrupts,
		Transcript:            cfg.stores.Transcript,
		Runs:                  cfg.stores.Runs,
		Snapshots:             sessionStores,
		MaterialSnapshots:     sessionStores,
		Writes:                sessionStores,
		TransientState:        execution.transientSessions,
		ExecutionReleaser:     execution.executor,
		Paths:                 workspaceadapter.Resolver{},
		Models:                modelCapabilities,
		DefaultModelSelection: execution.models.defaultSelection,
		Checkpoints:           workspaceServices.checkpoints,
		Admissions:            admissionGate,
		Invalidations:         policy.invalidations.publish,
		Now:                   time.Now,
		NewID:                 newSessionID,
		NewRunID:              newRunID,
		NewItemID:             newItemID,
	}
	sessionDependencies.Plan = sessions.PlanServices{
		Boundaries: cfg.stores.Plan, Replacements: policy.plans,
	}
	sessionDependencies.Mutations = cfg.stores.WorkspaceMutations
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
		Interrupts:          cfg.stores.Interrupts,
		Sessions:            cfg.stores.Sessions,
		Transcript:          cfg.stores.Transcript,
		ItemReplacer:        cfg.stores.Transcript,
		ModelInvocations:    cfg.stores.ModelInvocations,
		ToolInvocations:     cfg.stores.ToolInvocations,
		Conversation:        execution.conversation.store,
		Compactions:         execution.conversation.compactions,
		ContextState:        execution.transientSessions,
		State:               cfg.stores.Runs,
		RunProgress:         cfg.stores.Runs,
		ExecutorCheckpoints: cfg.stores.ExecutorCheckpoints,
		ExecutionTrees:      cfg.stores.ExecutorCheckpoints,
		ChildRunStarts:      cfg.stores.ChildRunStarts,
		Tx:                  segment.Transactor(cfg.stores.Transactor),
		PluginBindings: func(ctx context.Context, identity string, checkpoint runs.ExecutorCheckpoint, apply func(context.Context) error) error {
			return cfg.stores.CommitPluginReferences(ctx, policy.framework.host, identity, checkpoint, apply)
		},
	}
	runSegmentConfig.CommandAcceptances = cfg.stores.CommandAcceptances
	runSegmentConfig.ActorAuthority, err = plugins.NewRunActorAdmission(cfg.stores.Installations)
	if err != nil {
		return nil, err
	}
	runSegmentConfig.GoalRuns = goalRuns
	runSegmentConfig.ToolResults = cfg.stores.ToolResults
	runSegmentConfig.RememberedRules = cfg.stores.ApprovalRules
	runSegmentConfig.AcceptedContinuations = cfg.stores.AcceptedContinuations
	runSegmentEffects, err := segment.New(runSegmentConfig)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run-segment effects: %w", err)
	}
	runFinalizer, err := segment.NewFinalizer(segment.FinalizerConfig{
		Checkpoints: workspaceServices.checkpoints,
		Titles: &segment.TitleMaintenance{
			Sessions:  sessionCoordinator,
			Generator: segment.NewTitleGenerator(execution.models.utilityModel),
			Tasks:     runEffectTasks,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run finalizer: %w", err)
	}
	workspaceNotifier := segment.NewWorkspaceNotifier(fileChanges.publish)
	runDependencies := runs.Dependencies{
		CommandAcceptances:                 cfg.stores.CommandAcceptances,
		RootStarts:                         execution.executor,
		Observations:                       execution.executor,
		Releases:                           execution.executor,
		RootCancellation:                   execution.executor,
		Conversation:                       execution.conversation.messages,
		Models:                             modelCapabilities,
		ModelSelector:                      execution.tools.callComposition,
		Continuation:                       execution.executor,
		ApprovalRules:                      policy.approvals,
		AcceptedContinuations:              cfg.stores.AcceptedContinuations,
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
			Events:                      runSegmentEffects,
			Barriers:                    runSegmentEffects,
			WaitingSubtreeCancellations: runSegmentEffects,
			Workspace:                   workspaceNotifier,
			Finalizer:                   runFinalizer,
		},
		Runs:          cfg.stores.Runs,
		Items:         cfg.stores.Transcript,
		Admissions:    admissionGate,
		Now:           time.Now,
		NewRunID:      newRunID,
		NewSegmentID:  newSegmentID,
		Invalidations: policy.invalidations.publish,
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
	approvalCoordinator, err := approvals.New(policy.approvals, sessionCoordinator)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct approval coordinator: %w", err)
	}

	toolCoordinator, err := workspace.NewDiagnosticTools(execution.toolRegistry, workspaceServices.scope)
	if err != nil {
		return nil, fmt.Errorf("runtime: build diagnostic tools: %w", err)
	}

	mcpCoordinator, err := mcpapp.New(mcpapp.Config{
		Registry:      policy.registry,
		Store:         cfg.stores.MCPServers,
		Connections:   execution.tools.mcp,
		ToolCatalog:   execution.tools.tools.Resolver,
		Exposure:      policy.mcp.exposure,
		Invalidations: policy.invalidations.publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct MCP coordinator: %w", err)
	}
	lifetime.mcpCoordinator = mcpCoordinator
	pluginCoordinator, err := plugins.New(lifetime.context, policy.framework.host, cfg.stores.Installations, cfg.stores.PluginReleases, policy.packages, mcpCoordinator, policy.invalidations.publish)
	if err != nil {
		return nil, err
	}
	pluginSelections, err := plugins.NewSelections(cfg.stores.ProviderSelections, cfg.stores.Installations, execution.tools.admission, policy.invalidations.publish)
	if err != nil {
		return nil, err
	}
	pluginCompositions, err := plugins.NewCompositions(cfg.stores.PluginCompositions, cfg.stores.Installations, execution.tools.admission, policy.invalidations.publish)
	if err != nil {
		return nil, err
	}

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	goalDriver, err := goals.NewDriver(
		policy.goals,
		runCoordinator,
		cfg.stores.Sessions,
		goalMutations,
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
		Sessions:            cfg.stores.Sessions,
		Runs:                cfg.stores.Runs,
		Interrupts:          cfg.stores.Interrupts,
		Transcript:          cfg.stores.Transcript,
		Messages:            execution.conversation.store,
		GoalRuns:            goalRuns,
		ExecutorCheckpoints: cfg.stores.ExecutorCheckpoints,
		ModelInvocations:    cfg.stores.ModelInvocations,
		ToolInvocations:     cfg.stores.ToolInvocations,
		ChildRunStarts:      cfg.stores.ChildRunStarts,
		Tx:                  recovery.Transactor(cfg.stores.Transactor),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery persistence: %w", err)
	}
	bootRecovery, err := runs.NewRecovery(
		recoveryPersistence,
		execution.executor,
		admissionGate,
		policy.invalidations.publish,
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery: %w", err)
	}

	ownershipRecovery, err := ownership.NewRecovery(bootRecovery, goalDriver)
	if err != nil {
		return nil, fmt.Errorf("runtime: ownership recovery: %w", err)
	}
	workspaceFiles, err := workspace.NewFiles(workspaceServices.scope, workspaceadapter.FileBrowser{})
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace files: %w", err)
	}
	workspaceVCS, err := workspace.NewVCS(workspaceServices.scope, workspaceadapter.VCS{})
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace vcs: %w", err)
	}
	workspaceDiscovery, err := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{},
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace discovery: %w", err)
	}
	workspaceWatch, err := workspace.NewWatch(
		workspaceServices.scope,
		workspaceadapter.NewFileWatcher(lifetime.context),
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: build workspace git watch: %w", err)
	}
	modelInvocations, err := persistence.NewModelInvocationReader(cfg.stores.ModelInvocations)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct model invocation reader: %w", err)
	}
	modelEndpoints, err := models.NewEndpointObservations(cfg.stores.ModelInvocations)
	if err != nil {
		return nil, err
	}
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		ReadSnapshot:     cfg.stores.ReadSnapshot,
		Trajectory:       cfg.stores.Trajectory,
		ModelInvocations: modelInvocations,
		Transcript:       cfg.stores.Transcript,
		Interrupts:       cfg.stores.Interrupts,
		Runs:             cfg.stores.Runs,
		Sessions:         cfg.stores.Sessions,
		Plan:             cfg.stores.Plan,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct session queries: %w", err)
	}
	usage, err := sessions.NewUsageReporter(sessions.UsageDependencies{
		Source: cfg.stores.ModelInvocations,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct usage reporter: %w", err)
	}
	feedback, err := feedback.NewRecorder(cfg.stores.Feedback)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct feedback recorder: %w", err)
	}
	trajectoryReader, err := persistence.NewTrajectoryExportReader(persistence.TrajectoryExportConfig{
		Snapshots: sessionStores, Sessions: cfg.stores.Sessions, ModelInvocations: modelInvocations,
		ToolInvocations: cfg.stores.ToolInvocations,
		Feedback:        cfg.stores.Feedback, ReadSnapshot: persistence.Transactor(cfg.stores.ReadSnapshot),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct trajectory export reader: %w", err)
	}
	trajectoryExports, err := sessions.NewTrajectoryExporter(sessionCoordinator, trajectoryReader)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct trajectory exporter: %w", err)
	}
	// Startup effects run only after every constructor has succeeded, callback
	// RPC is published and original execution protection has been reconciled.
	var recoveryMu sync.Mutex
	recovered := false
	recoverUncertain := func(ctx context.Context) error {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if recovered {
			return nil
		}
		if err := cfg.stores.Admission.RecoveryError(); err != nil {
			return err
		}
		if _, err := cfg.stores.ToolResults.PurgeUnbound(ctx); err != nil {
			return fmt.Errorf("runtime: reconcile staged tool results: %w", err)
		}
		if err := pluginCoordinator.Reclaim(ctx); err != nil {
			return fmt.Errorf("runtime: reclaim plugin content: %w", err)
		}
		if err := runCoordinator.ReviveAcceptedContinuations(ctx); err != nil {
			return fmt.Errorf("runtime: revive accepted continuations: %w", err)
		}
		if err := cfg.stores.ModelInvocations.ReconcileEndpointInvocations(ctx, time.Now().UTC()); err != nil {
			return err
		}
		if err := runCoordinator.ReviveUncertainExecutions(ctx, recoveryPersistence, cfg.stores.ExecutorCheckpoints, execution.executor); err != nil {
			return fmt.Errorf("runtime: revive uncertain executions: %w", err)
		}
		if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
			return fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
		}
		recovered = true
		return nil
	}
	host := &Instance{
		application: &runtimeApplication{
			pluginControl: policy.framework.control,
			pluginsReady:  policy.framework.ready,
			delivery: delivery.HandlerConfig{
				Sessions:               sessionCoordinator,
				MCP:                    mcpCoordinator,
				Plugins:                pluginCoordinator,
				PluginSelections:       pluginSelections,
				PluginCompositions:     pluginCompositions,
				PluginCalls:            execution.tools.pluginCalls,
				PluginBrowser:          policy.framework.browser,
				Approvals:              approvalCoordinator,
				Models:                 modelCoordinator,
				ModelEndpoints:         modelEndpoints,
				Tools:                  toolCoordinator,
				Runs:                   runCoordinator,
				FileChanges:            fileChanges.observe,
				Invalidations:          policy.invalidations.observe,
				Queries:                queries,
				Usage:                  usage,
				Feedback:               feedback,
				TrajectoryExports:      trajectoryExports,
				WorkspaceFiles:         workspaceFiles,
				WorkspaceVCS:           workspaceVCS,
				WorkspaceDiscovery:     workspaceDiscovery,
				WorkspaceSkills:        workspaceServices.skills,
				Hooks:                  execution.hooks,
				WorkspaceWatch:         workspaceWatch,
				WorkspaceAuthoredWatch: workspaceServices.authoredWatch,
				Goals:                  goalDriver,
				GitAvailable:           git.Available(),
				IsolationAvailable:     sandbox.Available(),
			},
			sessions: sessionCoordinator,
			workers: runtimeWorkers{
				recovery:      ownershipRecovery,
				invalidations: policy.invalidations.publish,
			},
			idempotencyStore:           cfg.stores.Idempotency,
			callbacks:                  execution.models.callbacks,
			modelCallbacks:             execution.tools.modelCallbacks,
			recoverUncertainExecutions: recoverUncertain,
		},
		lifetime: lifetime,
	}
	return host, nil
}
