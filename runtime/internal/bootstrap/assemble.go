package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/agent/projects"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
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
		policy.invalidations.publish,
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
	projectCoordinator, err := projects.New(projects.Config{
		Store: cfg.stores.Projects, NewID: newProjectID, Now: time.Now, Invalidations: policy.invalidations.publish,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Project coordinator: %w", err)
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
				Projects:               projectCoordinator,
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
				recovery: ownershipRecovery,
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
		Projects:              cfg.stores.Projects,
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
