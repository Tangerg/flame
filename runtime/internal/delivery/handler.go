package delivery

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

// HandlerConfig declares the application use cases, notification sources, and
// contract facts required to construct a Handler.
type HandlerConfig struct {
	Sessions  sessionUseCases
	MCP       mcpUseCases
	Approvals approvalUseCases
	Models    modelUseCases
	Tools     toolUseCases
	Runs      runUseCases
	Queries   queryUseCases
	Usage     usageUseCases
	Feedback  feedbackUseCases

	FileChanges func(func(workspaceapp.FileChangeNotice))

	// ServerInfo identifies this runtime on the wire. Name and Version receive
	// development defaults when absent.
	ServerInfo protocol.ServerInfo
	// IdempotencyLimits is the replay window enforced for command retries.
	IdempotencyLimits protocol.IdempotencyLimits

	Schedules      scheduleManagementUseCases
	ScheduleFiring scheduleFiringUseCases
	Invalidations  func(func(invalidation.Notice))

	// Goals exposes the autonomous Goal use cases.
	Goals goalUseCases

	// AgentMemory is the HITL review use-case surface over the agent's
	// self-maintained memory (agentMemory.*).
	AgentMemory agentMemoryUseCases

	WorkspaceFiles         workspaceFileUseCases
	WorkspaceVCS           workspaceVCSUseCases
	WorkspaceDiscovery     workspaceDiscoveryUseCases
	WorkspaceKnowledge     workspaceKnowledgeUseCases
	WorkspaceSkills        workspaceSkillUseCases
	WorkspaceHooks         workspaceHookUseCases
	WorkspaceWatch         workspaceWatchUseCases
	WorkspaceAuthoredWatch workspaceAuthoredWatchUseCases

	GitAvailable bool
}

// Handler is the complete protocol-to-application translation target used by
// [Endpoint]. It is not an entrypoint or lifecycle owner: every production
// call and shutdown transition goes through the Endpoint.
type Handler struct {
	serverInfo protocol.ServerInfo

	sessions       sessionUseCases
	mcp            mcpUseCases
	approvals      approvalUseCases
	models         modelUseCases
	tools          toolUseCases
	runs           runUseCases
	queries        queryUseCases
	usage          usageUseCases
	feedback       feedbackUseCases
	schedules      scheduleManagementUseCases
	scheduleFiring scheduleFiringUseCases

	goals       goalUseCases
	agentMemory agentMemoryUseCases

	workspaceFiles         workspaceFileUseCases
	workspaceVCS           workspaceVCSUseCases
	workspaceDiscovery     workspaceDiscoveryUseCases
	workspaceKnowledge     workspaceKnowledgeUseCases
	workspaceSkills        workspaceSkillUseCases
	workspaceHooks         workspaceHookUseCases
	workspaceWatch         workspaceWatchUseCases
	workspaceAuthoredWatch workspaceAuthoredWatchUseCases

	features featureAvailability

	replay                   protocol.RunReplayLimits
	idempotency              protocol.IdempotencyLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits

	// workspaceHub fans non-run change signals out to
	// runtime.subscribe streams. It is ephemeral, lossy, and scoped
	// to this process; run streams have their own durable replay contract.
	workspaceHub *workspaceHub
}

// featureAvailability is the small closed set of optional runtime facts that
// shape both capability discovery and delivery gates. Construction derives it
// once; handlers do not rediscover availability by attempting a call.
type featureAvailability struct {
	git bool
}

// beginShutdown rejects new runtime subscriptions. Endpoint is its sole caller.
func (s *Handler) beginShutdown() {
	if s == nil {
		return
	}
	if s.workspaceHub != nil {
		s.workspaceHub.closeAdmissions()
	}
}

// NewHandler builds the protocol handler set from its required application use cases.
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
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{name: "Sessions", value: c.Sessions},
		{name: "MCP", value: c.MCP},
		{name: "Approvals", value: c.Approvals},
		{name: "Models", value: c.Models},
		{name: "Tools", value: c.Tools},
		{name: "Runs", value: c.Runs},
		{name: "Queries", value: c.Queries},
		{name: "Usage", value: c.Usage},
		{name: "Feedback", value: c.Feedback},
		{name: "Schedules", value: c.Schedules},
		{name: "ScheduleFiring", value: c.ScheduleFiring},
		{name: "AgentMemory", value: c.AgentMemory},
		{name: "Goals", value: c.Goals},
		{name: "WorkspaceFiles", value: c.WorkspaceFiles},
		{name: "WorkspaceVCS", value: c.WorkspaceVCS},
		{name: "WorkspaceDiscovery", value: c.WorkspaceDiscovery},
		{name: "WorkspaceKnowledge", value: c.WorkspaceKnowledge},
		{name: "WorkspaceSkills", value: c.WorkspaceSkills},
		{name: "WorkspaceHooks", value: c.WorkspaceHooks},
		{name: "WorkspaceWatch", value: c.WorkspaceWatch},
		{name: "WorkspaceAuthoredWatch", value: c.WorkspaceAuthoredWatch},
	} {
		if !capabilityAvailable(dependency.value) {
			return fmt.Errorf("delivery: %s is required", dependency.name)
		}
	}
	if _, err := runtimeidentity.ParseRuntimeInstance(c.ServerInfo.InstanceID); err != nil {
		return fmt.Errorf("delivery: ServerInfo.InstanceID: %w", err)
	}
	return nil
}

func (c HandlerConfig) withServerInfoDefaults() HandlerConfig {
	if c.ServerInfo.Name == "" {
		c.ServerInfo.Name = runtimeidentity.ProductName
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
			git: cfg.GitAvailable,
		},
		replay: replayLimitsFrom(cfg.Runs.ReplayRetention()),
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
	if _, err := runtimeidentity.ParseIdempotencyNamespace(cfg.IdempotencyLimits.Namespace); err != nil {
		return contractFacts{}, fmt.Errorf("delivery: IdempotencyLimits namespace: %w", err)
	}
	return facts, nil
}

func newHandler(cfg HandlerConfig, facts contractFacts) *Handler {
	return &Handler{
		sessions:                 cfg.Sessions,
		mcp:                      cfg.MCP,
		approvals:                cfg.Approvals,
		models:                   cfg.Models,
		tools:                    cfg.Tools,
		runs:                     cfg.Runs,
		queries:                  cfg.Queries,
		usage:                    cfg.Usage,
		feedback:                 cfg.Feedback,
		serverInfo:               cfg.ServerInfo,
		workspaceHub:             newWorkspaceHub(),
		schedules:                cfg.Schedules,
		scheduleFiring:           cfg.ScheduleFiring,
		goals:                    cfg.Goals,
		agentMemory:              cfg.AgentMemory,
		workspaceFiles:           cfg.WorkspaceFiles,
		workspaceVCS:             cfg.WorkspaceVCS,
		workspaceDiscovery:       cfg.WorkspaceDiscovery,
		workspaceKnowledge:       cfg.WorkspaceKnowledge,
		workspaceSkills:          cfg.WorkspaceSkills,
		workspaceHooks:           cfg.WorkspaceHooks,
		workspaceWatch:           cfg.WorkspaceWatch,
		workspaceAuthoredWatch:   cfg.WorkspaceAuthoredWatch,
		features:                 facts.features,
		replay:                   facts.replay,
		idempotency:              cfg.IdempotencyLimits,
		mcpAuthorizationAttempts: facts.mcpAuthorizationAttempts,
	}
}

func (s *Handler) observeNotificationSources(cfg HandlerConfig) {
	if cfg.FileChanges != nil {
		s.observeFileChanges(cfg.FileChanges)
	}
	if cfg.Invalidations != nil {
		s.observeInvalidations(cfg.Invalidations)
	}
}

// capabilities returns this Handler's capability snapshot. Its
// optional keys come from the same immutable composition facts that handlers
// use for their capability gates.
func (s *Handler) capabilities() protocol.ServerCapabilities {
	return capabilitiesFor(s.features, s.replay, s.idempotency, s.mcpAuthorizationAttempts)
}

// replayLimitsFrom captures the replay window the Runs use case enforces.
//
// The scope is named here because "which buffer a cursor can reach into" is wire
// vocabulary, and it holds by construction rather than by convention: a Journal
// is created per segment, and every cursor it mints carries that segment and this
// process — so a cursor from another segment is refused as invalid and one from
// another process as unavailable. Making those two refusals happen is what checks
// this claim.
func replayLimitsFrom(retention runs.Retention) protocol.RunReplayLimits {
	return protocol.RunReplayLimits{
		Scope:     protocol.ReplayScopeRuntimeInstanceRootSegment,
		MaxEvents: retention.MaxEvents,
		MaxBytes:  retention.MaxBytes,
	}
}

// capabilitiesFor builds the advertised contract from actual composition. A
// capability is never inferred from an RPC error; discovery and gating share
// the same facts so an advertised feature is callable and a disabled feature
// is absent before the client issues a request.
func capabilitiesFor(
	features featureAvailability,
	replay protocol.RunReplayLimits,
	idempotency protocol.IdempotencyLimits,
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits,
) protocol.ServerCapabilities {
	runEvents := []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentProgress,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamPlanUpdated,
	}
	return protocol.ServerCapabilities{
		RunEvents: runEvents,
		// The subscribable topics, read from the one closed list the subscribe request
		// is validated against. A second list here is how discovery comes to offer a
		// topic the runtime then refuses.
		RuntimeTopics: protocol.RuntimeTopics(),
		// The two bounds a client cannot discover by trying: what a reconnect can expect
		// to get back, and how wide one subscription may be.
		Limits: protocol.RuntimeLimits{
			Idempotency: idempotency,
			// No process-wide run cap is enforced, so maxConcurrentRuns stays absent
			// rather than advertising a limit the admission layer does not own.
			RunReplay:                replay,
			MCPAuthorizationAttempts: mcpAuthorizationAttempts,
			RuntimeSubscription: protocol.SubscriptionLimits{
				MaxTopics: protocol.MaxSubscriptionTopics, MaxWatches: protocol.MaxSubscriptionWatches,
			},
		},
		// The streaming methods, read from the registry that routes them. A
		// hand-kept list here would be a second author of "which calls stream" —
		// and the one clients trust, since this is what discovery advertises.
		StreamingMethods: Contract().StreamMethods(),
		// Open features map: a client treats an absent key as off. This is the
		// one composition fact per key — whether THIS build offers it — joined with
		// the feature's own published facts (opt-in and whether it reshapes
		// the run protocol), which come from protocol's registry. Advertising them
		// here by hand would let discovery promise a negotiation the runtime does
		// not perform.
		Features: advertisedFeatures(map[string]bool{
			protocol.FeatureReasoning: true,
			protocol.FeatureMCP:       true,
			protocol.FeatureKnowledge: true,
			protocol.FeatureSkills:    true,
			protocol.FeatureGit:       features.git,
			protocol.FeatureFileWatch: true,
			protocol.FeatureLSP:       true,

			protocol.FeatureSessionExport: true,
			// File checkpoints (restoreType on rollback) ride the shadow-git
			// store, which needs the git binary — same gate as the git feature.
			protocol.FeatureCheckpoints: features.git,
			protocol.FeatureMultimodal:  true,
			protocol.FeatureRelocate:    true,
			protocol.FeaturePlan:        true,
			protocol.FeatureCompaction:  true,
			protocol.FeatureGoals:       true,
			protocol.FeatureAgentMemory: true,
			protocol.FeatureSchedules:   true,
			protocol.FeatureSubagents:   true,
		}),
	}
}

// advertisedFeatures joins each published feature with this composition's answer
// to "do we offer it". Every key in the vocabulary is advertised — a client reads
// `enabled:false` and hides the surface, which is more useful than an absent key it
// has to guess about — and a build fact for a key no vocabulary defines is a
// programming error, since a client could never ask about it.
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
			Enabled:               enabled[feature.Key],
			ClientOptIn:           feature.ClientOptIn,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	return out
}

// ─── helpers ────────────────────────────────────────────────────────

// capabilityNotNegotiated marks a protocol method that exists in the contract
// but isn't backed on this build. Maps to capability_not_negotiated
// — consistent with the feature flag advertised through discovery.
func capabilityNotNegotiated(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
