package delivery

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/feedback"
	"github.com/Tangerg/flame/runtime/internal/application/agent/projects"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type HandlerConfig struct {
	Sessions           *sessions.Coordinator
	MCP                *mcpapp.Coordinator
	Plugins            *plugins.Coordinator
	PluginSelections   *plugins.Selections
	PluginCompositions *plugins.Compositions
	PluginCalls        pluginCallUseCases
	PluginBrowser      pluginbrowser.Service
	Approvals          *approvals.RuntimePolicy
	Models             *modelapp.Coordinator
	ModelEndpoints     *modelapp.EndpointObservations
	Tools              *workspaceapp.DiagnosticTools
	Runs               *runs.Coordinator
	RunOpenings        *runs.Openings
	RunCancellations   *runs.Cancellations
	RunStreams         *runs.Segments
	Queries            *sessions.QueryCoordinator
	Projects           *projects.Coordinator
	Usage              *sessions.UsageReporter
	Feedback           *feedback.Recorder
	TrajectoryExports  *sessions.TrajectoryExporter

	FileChanges func(func(workspaceapp.FileChangeNotice))

	ServerInfo        protocol.ServerInfo
	IdempotencyLimits protocol.IdempotencyLimits

	Invalidations func(func(invalidation.Notice))

	Goals *goals.Driver

	WorkspaceFiles         *workspaceapp.Files
	WorkspaceVCS           *workspaceapp.VCS
	WorkspaceDiscovery     *workspaceapp.Discovery
	WorkspaceSkills        *workspaceapp.Skills
	Hooks                  *apphooks.Catalog
	WorkspaceWatch         *workspaceapp.Watch
	WorkspaceAuthoredWatch *workspaceapp.AuthoredWatch

	GitAvailable       bool
	IsolationAvailable bool
}

type Handler struct {
	serverInfo protocol.ServerInfo

	sessions           *sessions.Coordinator
	mcp                *mcpapp.Coordinator
	plugins            *plugins.Coordinator
	pluginSelections   *plugins.Selections
	pluginCompositions *plugins.Compositions
	pluginCalls        pluginCallUseCases
	pluginBrowser      pluginbrowser.Service
	approvals          *approvals.RuntimePolicy
	models             *modelapp.Coordinator
	modelEndpoints     *modelapp.EndpointObservations
	tools              *workspaceapp.DiagnosticTools
	runs               *runs.Coordinator
	runOpenings        *runs.Openings
	runCancellations   *runs.Cancellations
	runStreams         *runs.Segments
	queries            *sessions.QueryCoordinator
	projects           *projects.Coordinator
	usage              *sessions.UsageReporter
	feedback           *feedback.Recorder
	trajectoryExports  *sessions.TrajectoryExporter

	goals *goals.Driver

	workspaceFiles         *workspaceapp.Files
	workspaceVCS           *workspaceapp.VCS
	workspaceDiscovery     *workspaceapp.Discovery
	workspaceSkills        *workspaceapp.Skills
	hooks                  *apphooks.Catalog
	workspaceWatch         *workspaceapp.Watch
	workspaceAuthoredWatch *workspaceapp.AuthoredWatch

	features featureAvailability

	replay                   protocol.RunReplayLimits
	idempotency              protocol.IdempotencyLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits

	runtimeEvents *runtimeEventHub
}

type featureAvailability struct {
	git       bool
	isolation bool
}

func (s *Handler) beginShutdown() {
	s.runtimeEvents.closeAdmissions()
}

func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withServerInfoDefaults()
	facts, err := deriveContractFacts(cfg)
	if err != nil {
		return nil, err
	}
	handler := newHandler(cfg, facts)
	handler.observeNotificationSources(cfg)
	return handler, nil
}

func (c HandlerConfig) validate() error {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "Sessions", value: c.Sessions},
		{name: "MCP", value: c.MCP},
		{name: "Plugins", value: c.Plugins},
		{name: "PluginSelections", value: c.PluginSelections},
		{name: "PluginCompositions", value: c.PluginCompositions},
		{name: "PluginCalls", value: c.PluginCalls},
		{name: "PluginBrowser", value: c.PluginBrowser},
		{name: "Approvals", value: c.Approvals},
		{name: "Models", value: c.Models},
		{name: "Tools", value: c.Tools},
		{name: "Runs", value: c.Runs},
		{name: "RunOpenings", value: c.RunOpenings},
		{name: "RunCancellations", value: c.RunCancellations},
		{name: "Queries", value: c.Queries},
		{name: "Projects", value: c.Projects},
		{name: "Usage", value: c.Usage},
		{name: "Feedback", value: c.Feedback},
		{name: "TrajectoryExports", value: c.TrajectoryExports},
		{name: "Goals", value: c.Goals},
		{name: "WorkspaceFiles", value: c.WorkspaceFiles},
		{name: "WorkspaceVCS", value: c.WorkspaceVCS},
		{name: "WorkspaceDiscovery", value: c.WorkspaceDiscovery},
		{name: "WorkspaceSkills", value: c.WorkspaceSkills},
		{name: "Hooks", value: c.Hooks},
		{name: "WorkspaceWatch", value: c.WorkspaceWatch},
		{name: "WorkspaceAuthoredWatch", value: c.WorkspaceAuthoredWatch},
		{name: "FileChanges", value: c.FileChanges},
		{name: "Invalidations", value: c.Invalidations},
	} {
		if !capabilityAvailable(required.value) {
			return fmt.Errorf("delivery: %s is required", required.name)
		}
	}
	if err := ValidateInstanceID(c.ServerInfo.InstanceID); err != nil {
		return fmt.Errorf("delivery: ServerInfo.InstanceID: %w", err)
	}
	return nil
}

func (c HandlerConfig) withServerInfoDefaults() HandlerConfig {
	if c.ServerInfo.Name == "" {
		c.ServerInfo.Name = identity.ProductName
	}
	if c.ServerInfo.Version == "" {
		c.ServerInfo.Version = "0.0.0-dev"
	}
	return c
}

type contractFacts struct {
	features                 featureAvailability
	replay                   protocol.RunReplayLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits
}

func deriveContractFacts(cfg HandlerConfig) (contractFacts, error) {
	facts := contractFacts{
		features: featureAvailability{
			git:       cfg.GitAvailable,
			isolation: cfg.IsolationAvailable,
		},
		replay: replayLimitsFrom(cfg.RunStreams.ReplayRetention()),
		mcpAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{
			RetentionSeconds: int(cfg.MCP.AuthorizationAttemptRetention().Seconds()),
		},
	}
	for _, wireShape := range []struct {
		label string
		value any
	}{
		{label: "Runs returned invalid replay retention", value: facts.replay},
		{label: "IdempotencyLimits is invalid", value: cfg.IdempotencyLimits},
		{label: "MCP returned invalid authorization attempt retention", value: facts.mcpAuthorizationAttempts},
	} {
		if err := protocol.ValidateWireTree(wireShape.value); err != nil {
			return contractFacts{}, fmt.Errorf("delivery: %s: %w", wireShape.label, err)
		}
	}
	if _, err := operation.ParseReplayNamespace(cfg.IdempotencyLimits.Namespace); err != nil {
		return contractFacts{}, fmt.Errorf("delivery: IdempotencyLimits namespace: %w", err)
	}
	return facts, nil
}

func newHandler(cfg HandlerConfig, facts contractFacts) *Handler {
	return &Handler{
		sessions:                 cfg.Sessions,
		mcp:                      cfg.MCP,
		plugins:                  cfg.Plugins,
		pluginSelections:         cfg.PluginSelections,
		pluginCompositions:       cfg.PluginCompositions,
		pluginCalls:              cfg.PluginCalls,
		pluginBrowser:            cfg.PluginBrowser,
		approvals:                cfg.Approvals,
		models:                   cfg.Models,
		modelEndpoints:           cfg.ModelEndpoints,
		tools:                    cfg.Tools,
		runs:                     cfg.Runs,
		runOpenings:              cfg.RunOpenings,
		runCancellations:         cfg.RunCancellations,
		runStreams:               cfg.RunStreams,
		queries:                  cfg.Queries,
		projects:                 cfg.Projects,
		usage:                    cfg.Usage,
		feedback:                 cfg.Feedback,
		trajectoryExports:        cfg.TrajectoryExports,
		serverInfo:               cfg.ServerInfo,
		runtimeEvents:            newRuntimeEventHub(),
		goals:                    cfg.Goals,
		workspaceFiles:           cfg.WorkspaceFiles,
		workspaceVCS:             cfg.WorkspaceVCS,
		workspaceDiscovery:       cfg.WorkspaceDiscovery,
		workspaceSkills:          cfg.WorkspaceSkills,
		hooks:                    cfg.Hooks,
		workspaceWatch:           cfg.WorkspaceWatch,
		workspaceAuthoredWatch:   cfg.WorkspaceAuthoredWatch,
		features:                 facts.features,
		replay:                   facts.replay,
		idempotency:              cfg.IdempotencyLimits,
		mcpAuthorizationAttempts: facts.mcpAuthorizationAttempts,
	}
}

func (s *Handler) observeNotificationSources(cfg HandlerConfig) {
	s.observeFileChanges(cfg.FileChanges)
	s.observeInvalidations(cfg.Invalidations)
}

func (s *Handler) capabilities() protocol.ServerCapabilities {
	return capabilitiesFor(s.features, s.replay, s.idempotency, s.mcpAuthorizationAttempts)
}

func replayLimitsFrom(retention runs.Retention) protocol.RunReplayLimits {
	return protocol.RunReplayLimits{
		Scope:     protocol.ReplayScopeRuntimeInstanceRootSegment,
		MaxEvents: retention.MaxEvents,
		MaxBytes:  retention.MaxBytes,
	}
}

func capabilitiesFor(
	features featureAvailability,
	replay protocol.RunReplayLimits,
	idempotency protocol.IdempotencyLimits,
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits,
) protocol.ServerCapabilities {
	runEvents := []protocol.StreamEventType{protocol.StreamSegmentStarted, protocol.StreamSegmentProgress, protocol.StreamSegmentFinished, protocol.StreamItemStarted, protocol.StreamItemDelta, protocol.StreamItemCompleted, protocol.StreamPlanUpdated}
	return protocol.ServerCapabilities{
		RunEvents:     runEvents,
		RuntimeTopics: protocol.RuntimeTopics(),
		Limits: protocol.RuntimeLimits{
			Idempotency:              idempotency,
			RunReplay:                replay,
			MCPAuthorizationAttempts: mcpAuthorizationAttempts,
			RuntimeSubscription: protocol.SubscriptionLimits{
				MaxTopics: protocol.MaxSubscriptionTopics, MaxWatches: protocol.MaxSubscriptionWatches,
				MaxPaths: workspaceapp.MaxWatchPaths, MaxDirectoryEntries: workspaceapp.MaxWatchDirectoryEntries, MaxFileBytes: workspaceapp.MaxWatchFileBytes,
			},
		},
		StreamingMethods: Contract().StreamMethods(),
		Features: advertisedFeatures(map[string]bool{
			protocol.FeatureReasoning: true, protocol.FeatureMCP: true, protocol.FeaturePlugins: true,
			protocol.FeatureSkills: true, protocol.FeatureFileWatch: true, protocol.FeatureSessionExport: true,
			protocol.FeatureMultimodal: true, protocol.FeatureRelocate: true, protocol.FeaturePlan: true,
			protocol.FeatureCompaction: true, protocol.FeatureGoals: true, protocol.FeatureChildRuns: true,
			protocol.FeatureGit:         features.git,
			protocol.FeatureCheckpoints: features.git,
			protocol.FeatureIsolation:   features.isolation,
		}),
	}
}

func advertisedFeatures(enabled map[string]bool) map[string]protocol.FeatureCapability {
	for key := range enabled {
		if _, published := protocol.LookupFeature(key); !published {
			panic("delivery: composition advertises unpublished feature " + key)
		}
	}
	features := protocol.Features()
	out := make(map[string]protocol.FeatureCapability, len(features))
	for _, feature := range features {
		out[feature.Key] = protocol.FeatureCapability{
			Enabled:       enabled[feature.Key],
			ClientOptIn:   feature.ClientOptIn,
			RequiredByRun: feature.RequiredByRun,
		}
	}
	return out
}
