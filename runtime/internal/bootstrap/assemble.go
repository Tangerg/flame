package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/segment"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
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
	callbacks := pluginmcp.NewCallbacks()
	lifetime.toolResources = append(lifetime.toolResources, terminalStep(func(context.Context) error { return callbacks.Close() }))
	policy, err := buildPolicyComposition(ctx, cfg, lifetime, callbacks)
	if err != nil {
		return nil, err
	}
	workspaceServices, err := buildWorkspaceComposition(cfg, policy.invalidations.publish)
	if err != nil {
		return nil, err
	}
	execution, err := buildExecutionComposition(
		ctx,
		cfg,
		callbacks,
		lifetime,
		buildTools,
		policy,
		workspaceServices,
	)
	if err != nil {
		return nil, err
	}
	return buildAssemblyCore(cfg, lifetime, policy, workspaceServices, execution)
}

func buildAssemblyCore(
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
) (*Instance, error) {
	fileChanges := newNotificationRelay[workspace.FileChangeNotice]()
	goalRuns, err := goals.NewRunSettler(cfg.stores.Goals, policy.invalidations.publish)
	if err != nil {
		return nil, fmt.Errorf("runtime: Goal Run settler: %w", err)
	}
	sessionParts, err := assembleSessions(cfg, policy, workspaceServices, execution)
	if err != nil {
		return nil, err
	}
	modelCoordinator, err := assembleModels(cfg, policy, execution)
	if err != nil {
		return nil, err
	}
	retirer, admissionGate, sessionStores := sessionParts.retirer, sessionParts.admissions, sessionParts.stores
	sessionCoordinator, goalMutations := sessionParts.coordinator, sessionParts.goalMutations
	runUseCases, err := assembleRuns(cfg, lifetime, policy, workspaceServices, execution, runFoundation{
		sessions: sessionCoordinator, admissions: admissionGate, fileChanges: fileChanges.publish,
	})
	if err != nil {
		return nil, err
	}
	runCoordinator := runUseCases.coordinator
	lifetime.runSegments = runUseCases.segments

	toolCoordinator, err := workspace.NewDiagnosticTools(execution.toolRegistry, workspaceServices.scope)
	if err != nil {
		return nil, fmt.Errorf("runtime: build diagnostic tools: %w", err)
	}

	coordinators, err := assembleIntegrations(cfg, lifetime, policy, execution)
	if err != nil {
		return nil, err
	}
	mcpCoordinator, pluginCoordinator := coordinators.mcp, coordinators.plugins

	goalDriver, err := goals.NewDriver(
		policy.goals,
		runUseCases.openings,
		runUseCases.cancellations,
		cfg.stores.Sessions,
		goalMutations,
		execution.tools.pluginCalls, func(ctx context.Context, sessionID identity.SessionID) error {
			return goalRuns.Drain(ctx, policy.goals, execution.tools.pluginCalls, sessionID)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Goal driver: %w", err)
	}
	lifetime.goalDriver = goalDriver
	// Must precede Run recovery: it is part of the Deployment configuration that
	// validates a durable executor checkpoint.
	createGoalTool, newCreateErr := builtin.NewCreate(goalDriver)
	if newCreateErr != nil {
		return nil, fmt.Errorf("runtime: build create_goal: %w", newCreateErr)
	}
	execution.tools.tools.Resolver.UseCreateGoalTool(createGoalTool)

	recoveryPersistence, ownershipRecovery, err := assembleBootRecovery(cfg, execution, runUseCases.cancellations, admissionGate, goalDriver, policy)
	if err != nil {
		return nil, err
	}
	readServices, err := assembleReadServices(cfg, lifetime, workspaceServices, execution, sessionCoordinator, sessionStores)
	if err != nil {
		return nil, err
	}
	recoverUncertain := startupRunRecovery(cfg.stores, pluginCoordinator, runCoordinator, recoveryPersistence, execution.executor, ownershipRecovery)
	host := &Instance{
		application: &runtimeApplication{
			pluginControl: policy.framework.control,
			pluginsReady:  policy.framework.ready,
			delivery: delivery.HandlerConfig{
				Sessions:               sessionCoordinator,
				MCP:                    mcpCoordinator,
				Plugins:                pluginCoordinator,
				PluginSelections:       coordinators.selections,
				PluginCompositions:     coordinators.compositions,
				PluginCalls:            execution.tools.pluginCalls,
				PluginBrowser:          policy.framework.browser,
				Approvals:              policy.approvals,
				Models:                 modelCoordinator,
				ModelEndpoints:         readServices.modelEndpoints,
				Tools:                  toolCoordinator,
				Runs:                   runCoordinator,
				RunOpenings:            runUseCases.openings,
				RunCancellations:       runUseCases.cancellations,
				RunStreams:             runUseCases.segments,
				FileChanges:            fileChanges.observe,
				Invalidations:          policy.invalidations.observe,
				Queries:                readServices.queries,
				Usage:                  readServices.usage,
				Feedback:               readServices.feedback,
				TrajectoryExports:      readServices.trajectoryExports,
				WorkspaceFiles:         readServices.workspaceFiles,
				WorkspaceVCS:           readServices.workspaceVCS,
				WorkspaceDiscovery:     readServices.workspaceDiscovery,
				WorkspaceSkills:        execution.tools.skills,
				Hooks:                  execution.hooks,
				WorkspaceWatch:         readServices.workspaceWatch,
				WorkspaceAuthoredWatch: workspaceServices.authoredWatch,
				Goals:                  goalDriver,
				GitAvailable:           git.Available(),
				IsolationAvailable:     sandbox.Available(),
			},
			sessions:   sessionCoordinator,
			retirer:    retirer,
			admissions: admissionGate,
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
	mcpCoordinator.RestoreExternal(lifetime.context, policy.framework.ready)
	return host, nil
}

type sessionAssembly struct {
	retirer       *sessions.ResourceRetirer
	admissions    *ownership.Gate
	stores        *persistence.SessionStores
	coordinator   *sessions.Coordinator
	goalMutations *goals.SessionMutations
}

func assembleSessions(cfg assemblyConfig, policy policyComposition, workspaceServices workspaceComposition, execution executionComposition) (sessionAssembly, error) {
	retirer, err := sessions.NewResourceRetirer(cfg.stores.ResourceRetirements, sessionResourceCleanups(workspaceServices.checkpoints, execution.isolation)...)
	if err != nil {
		return sessionAssembly{}, fmt.Errorf("runtime: session resource retirement: %w", err)
	}
	admissionGate, err := ownership.NewGate(ownership.GateConfig{
		TreesOverlap: filesystem.PathsOverlap, ResolveTree: filesystem.CanonicalPath,
		SettleRetirement: retirer.Settle, WorkspaceRollbackPending: cfg.stores.WorkspaceMutations.SessionPending,
	})
	if err != nil {
		return sessionAssembly{}, fmt.Errorf("runtime: session admission: %w", err)
	}
	sessionStores, err := cfg.stores.SessionStores(execution.conversation.messages)
	if err != nil {
		return sessionAssembly{}, fmt.Errorf("runtime: session write-set stores: %w", err)
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
		Models:                execution.models.capabilities,
		DefaultModelSelection: cfg.defaultSelection,
		Checkpoints:           workspaceServices.checkpoints,
		Admissions:            admissionGate,
		Invalidations:         policy.invalidations.publish,
		RollbackReceipts:      delivery.EncodeWorkspaceRollbackReceipt,
		Now:                   time.Now,
		NewID:                 newSessionID,
		NewRunID:              newRunID,
		NewItemID:             newItemID,
	}
	sessionDependencies.Plan = sessions.PlanServices{
		Boundaries: cfg.stores.Plan, Replacements: execution.tools.plans,
	}
	sessionDependencies.Mutations = cfg.stores.WorkspaceMutations
	sessionDependencies.Retirer = retirer
	sessionDependencies.Sandbox = execution.isolation
	goalMutations := goals.NewSessionMutations()
	sessionDependencies.Goals = goalMutations
	sessionCoordinator, err := sessions.New(sessionDependencies)
	if err != nil {
		return sessionAssembly{}, fmt.Errorf("runtime: construct Session coordinator: %w", err)
	}
	return sessionAssembly{retirer: retirer, admissions: admissionGate, stores: sessionStores, coordinator: sessionCoordinator, goalMutations: goalMutations}, nil
}

func assembleModels(cfg assemblyConfig, policy policyComposition, execution executionComposition) (*models.Coordinator, error) {
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
	return modelCoordinator, nil
}

type integrationAssembly struct {
	mcp          *mcpapp.Coordinator
	plugins      *plugins.Coordinator
	selections   *plugins.Selections
	compositions *plugins.Compositions
}

func assembleIntegrations(cfg assemblyConfig, lifetime *runtimeLifetime, policy policyComposition, execution executionComposition) (integrationAssembly, error) {
	mcpCoordinator, err := mcpapp.New(mcpapp.Config{
		Registry:      policy.registry,
		Store:         policy.framework.external,
		External:      policy.framework.external,
		Connections:   execution.tools.mcp,
		ToolCatalog:   execution.tools.tools.Resolver,
		Exposure:      policy.mcp.exposure,
		Invalidations: policy.invalidations.publish,
	})
	if err != nil {
		return integrationAssembly{}, fmt.Errorf("runtime: construct MCP coordinator: %w", err)
	}
	lifetime.mcpCoordinator = mcpCoordinator
	pluginCoordinator, err := plugins.New(lifetime.context, policy.framework.host, cfg.stores.Installations, cfg.stores.PluginReleases, policy.packages, mcpCoordinator, cfg.stores.Transactor, policy.invalidations.publish)
	if err != nil {
		return integrationAssembly{}, err
	}
	pluginSelections, err := plugins.NewSelections(cfg.stores.ProviderSelections, cfg.stores.Installations, cfg.stores.Transactor, policy.invalidations.publish, mcpCoordinator)
	if err != nil {
		return integrationAssembly{}, err
	}
	pluginCompositions, err := plugins.NewCompositions(cfg.stores.PluginCompositions, cfg.stores.Installations, cfg.stores.Transactor, policy.invalidations.publish)
	if err != nil {
		return integrationAssembly{}, err
	}

	return integrationAssembly{mcp: mcpCoordinator, plugins: pluginCoordinator, selections: pluginSelections, compositions: pluginCompositions}, nil
}

func assembleBootRecovery(cfg assemblyConfig, execution executionComposition, cancellations *runs.Cancellations, admissionGate *ownership.Gate, goalDriver *goals.Driver, policy policyComposition) (*persistence.RecoveryStore, *ownership.RecoveryCoordinator, error) {
	recoveryPersistence := cfg.stores.Recovery()
	bootRecovery, err := runs.NewRecovery(
		recoveryPersistence,
		cancellations,
		execution.executor,
		admissionGate,
		policy.invalidations.publish,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: boot recovery: %w", err)
	}

	ownershipRecovery, err := ownership.NewRecovery(bootRecovery, goalDriver)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: ownership recovery: %w", err)
	}
	return recoveryPersistence, ownershipRecovery, nil
}

func sessionResourceCleanups(checkpoints *workspaceadapter.Checkpoints, isolation workspaceIsolation) []sessions.Cleanup {
	return []sessions.Cleanup{
		{Resource: "checkpoints", Retire: checkpoints.DropSession},
		{Resource: "sandbox", Retire: func(_ context.Context, id identity.SessionID) error { return isolation.Discard(id) }},
	}
}

type readServices struct {
	workspaceFiles     *workspace.Files
	workspaceVCS       *workspace.VCS
	workspaceDiscovery *workspace.Discovery
	workspaceWatch     *workspace.Watch
	modelEndpoints     *models.EndpointObservations
	queries            *sessions.QueryCoordinator
	usage              *sessions.UsageReporter
	feedback           *feedback.Recorder
	trajectoryExports  *sessions.TrajectoryExporter
}

func assembleReadServices(
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	workspaceServices workspaceComposition,
	execution executionComposition,
	sessionCoordinator *sessions.Coordinator,
	sessionStores *persistence.SessionStores,
) (readServices, error) {
	workspaceFiles, err := workspace.NewFiles(workspaceServices.scope, workspaceadapter.FileBrowser{})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace files: %w", err)
	}
	workspaceVCS, err := workspace.NewVCS(workspaceServices.scope, workspaceadapter.VCS{})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace vcs: %w", err)
	}
	workspaceDiscovery, err := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{},
	)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace discovery: %w", err)
	}
	workspaceWatch, err := workspace.NewWatch(
		workspaceServices.scope,
		workspaceadapter.NewFileWatcher(lifetime.context),
	)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: build workspace git watch: %w", err)
	}
	modelEndpoints, err := models.NewEndpointObservations(cfg.stores.ModelInvocations)
	if err != nil {
		return readServices{}, err
	}
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		ReadSnapshot:     cfg.stores.ReadSnapshot,
		Trajectory:       cfg.stores.Trajectory,
		ModelInvocations: cfg.stores.ModelInvocations,
		Transcript:       cfg.stores.Transcript,
		Interrupts:       cfg.stores.Interrupts,
		Runs:             cfg.stores.Runs,
		Sessions:         cfg.stores.Sessions,
		Plan:             cfg.stores.Plan,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct session queries: %w", err)
	}
	usage, err := sessions.NewUsageReporter(sessions.UsageDependencies{
		Source: cfg.stores.ModelInvocations, Now: time.Now,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct usage reporter: %w", err)
	}
	recorder, err := feedback.NewRecorder(execution.tools.pluginCalls, cfg.stores.Annotations, time.Now)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct feedback recorder: %w", err)
	}
	trajectoryReader, err := persistence.NewTrajectoryExportReader(persistence.TrajectoryExportConfig{
		Snapshots: sessionStores, Sessions: cfg.stores.Sessions, ModelInvocations: cfg.stores.ModelInvocations,
		ToolInvocations: cfg.stores.ToolInvocations,
		Annotations:     cfg.stores.Annotations, ReadSnapshot: cfg.stores.ReadSnapshot,
	})
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct trajectory export reader: %w", err)
	}
	trajectoryExports, err := sessions.NewTrajectoryExporter(sessionCoordinator, trajectoryReader)
	if err != nil {
		return readServices{}, fmt.Errorf("runtime: construct trajectory exporter: %w", err)
	}
	return readServices{
		workspaceFiles: workspaceFiles, workspaceVCS: workspaceVCS, workspaceDiscovery: workspaceDiscovery, workspaceWatch: workspaceWatch,
		modelEndpoints: modelEndpoints, queries: queries, usage: usage, feedback: recorder, trajectoryExports: trajectoryExports,
	}, nil
}

type runFoundation struct {
	sessions    *sessions.Coordinator
	admissions  *ownership.Gate
	fileChanges func(workspace.FileChangeNotice)
}

func assembleRuns(
	cfg assemblyConfig,
	lifetime *runtimeLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
	foundation runFoundation,
) (composedRuns, error) {
	var err error
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
		return composedRuns{}, err
	}
	runSegmentConfig.GoalRuns = cfg.stores.Goals
	runSegmentConfig.RememberedRules = cfg.stores.ApprovalRules
	runSegmentConfig.AcceptedContinuations = cfg.stores.AcceptedContinuations
	runSegmentEffects, err := segment.New(runSegmentConfig)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run-segment effects: %w", err)
	}
	runFinalizer, err := segment.NewFinalizer(segment.FinalizerConfig{
		Checkpoints: workspaceServices.checkpoints,
		Titles: segment.TitleMaintenance{
			Sessions: foundation.sessions,
			Model:    execution.models.utilityModel,
			Tasks:    runEffectTasks,
		},
	})
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run finalizer: %w", err)
	}
	workspaceNotifier, err := segment.NewWorkspaceNotifier(foundation.fileChanges)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct workspace notifier: %w", err)
	}
	runSegmentDependencies := runs.SegmentDependencies{
		Observations: execution.executor,
		Releases:     execution.executor,
		Publication: runs.PublicationPorts{
			Openings: runSegmentEffects, Events: runSegmentEffects, Barriers: runSegmentEffects, Workspace: workspaceNotifier,
		},
		ChildStarts:   runSegmentEffects,
		Finalizer:     runFinalizer,
		Runs:          cfg.stores.Runs,
		ToolAttempts:  cfg.stores.ToolInvocations,
		Admissions:    foundation.admissions,
		Now:           time.Now,
		Retention:     runs.DefaultRetention(),
		NewRunID:      newRunID,
		NewSegmentID:  newSegmentID,
		Invalidations: policy.invalidations.publish,
	}
	runDependencies := runs.Dependencies{
		CommandAcceptances:    cfg.stores.CommandAcceptances,
		Continuation:          execution.executor,
		ApprovalRules:         policy.approvals,
		AcceptedContinuations: cfg.stores.AcceptedContinuations,
		Steering:              execution.executor,
		NewItemID:             newItemID,
		Sessions:              foundation.sessions,
		Interrupts:            foundation.sessions,
		Checkpoints:           runSegmentEffects,
		Runs:                  cfg.stores.Runs,
		Items:                 cfg.stores.Transcript,
		Admissions:            foundation.admissions,
	}
	openingDependencies := runs.OpeningDependencies{
		CommandAcceptances: cfg.stores.CommandAcceptances,
		RootStarts:         execution.executor,
		Releases:           execution.executor,
		Conversation:       execution.conversation.messages,
		WorkingContexts:    execution.workingContexts,
		Models:             execution.models.capabilities,
		ModelSelector:      execution.tools.callComposition,
		Sessions:           foundation.sessions,
		SessionCreator:     foundation.sessions,
		ActiveRuns:         foundation.sessions,
		Admissions:         foundation.admissions,
	}
	cancellationDependencies := runs.CancellationDependencies{
		RootCancellation:                   execution.executor,
		RunningSubtreeCanceler:             execution.executor,
		WaitingSubtreeCancellationPreparer: execution.executor,
		WaitingSubtreeCancellations:        runSegmentEffects,
		AcceptedSubtreeCancellations:       cfg.stores.AcceptedContinuations,
		SubtreeCancellationRestorer:        execution.executor,
		Terminations:                       foundation.sessions,
		Sessions:                           foundation.sessions,
		Interrupts:                         foundation.sessions,
		Checkpoints:                        runSegmentEffects,
		Releases:                           execution.executor,
		Runs:                               cfg.stores.Runs,
		Items:                              cfg.stores.Transcript,
		Admissions:                         foundation.admissions,
	}
	openingDependencies.Isolation = execution.isolation
	return composeRuns(runDependencies, openingDependencies, cancellationDependencies, runSegmentDependencies)
}

type composedRuns struct {
	coordinator   *runs.Coordinator
	openings      *runs.Openings
	cancellations *runs.Cancellations
	segments      *runs.Segments
}

func composeRuns(
	deps runs.Dependencies,
	openingDeps runs.OpeningDependencies,
	cancellationDeps runs.CancellationDependencies,
	segmentDeps runs.SegmentDependencies,
) (composedRuns, error) {
	segments, err := runs.NewSegments(segmentDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run segments: %w", err)
	}
	deps.Segments, openingDeps.Segments, cancellationDeps.Segments = segments, segments, segments
	coordinator, err := runs.NewCoordinator(deps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run coordinator: %w", err)
	}
	openings, err := runs.NewOpenings(openingDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run openings: %w", err)
	}
	cancellations, err := runs.NewCancellations(cancellationDeps)
	if err != nil {
		return composedRuns{}, fmt.Errorf("runtime: construct Run cancellations: %w", err)
	}
	return composedRuns{coordinator: coordinator, openings: openings, cancellations: cancellations, segments: segments}, nil
}

func startupRunRecovery(stores *persistence.Bundle, pluginCoordinator *plugins.Coordinator, runCoordinator *runs.Coordinator, recoveryPersistence *persistence.RecoveryStore, executor *executionadapter.InteractionExecutor, ownershipRecovery *ownership.RecoveryCoordinator) func(context.Context) error {
	var recoveryMu sync.Mutex
	recovered := false
	prepared := false
	var uncertain runs.UncertainRevival
	return func(ctx context.Context) error {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if recovered {
			return nil
		}
		if err := stores.Admission.RecoveryError(); err != nil {
			return err
		}
		if !prepared {
			if err := pluginCoordinator.Reclaim(ctx); err != nil {
				return fmt.Errorf("runtime: reclaim plugin content: %w", err)
			}
			if err := runCoordinator.ReviveAcceptedContinuations(ctx); err != nil {
				return fmt.Errorf("runtime: revive accepted continuations: %w", err)
			}
			if err := stores.ModelInvocations.ReconcileEndpointInvocations(ctx, time.Now().UTC()); err != nil {
				return err
			}
			prepared = true
		}
		err := uncertain.Reconcile(ctx, runCoordinator, recoveryPersistence, executor)
		if err != nil && !errors.Is(err, runs.ErrOriginalSourcePending) {
			return fmt.Errorf("runtime: revive uncertain executions: %w", err)
		}
		if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
			return fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
		}
		if err != nil {
			return err
		}
		recovered = true
		return nil
	}
}
