package protocol

// There is deliberately no list of renderable event types: a client that cannot
// follow the authoritative stream cannot be served a shortened one.
type ClientCapabilities struct {
	Features                map[string]FeaturePreference `json:"features,omitempty"`
	InterruptTypes          []InterruptType              `json:"interruptTypes,omitempty"`
	ExcludedEphemeralEvents []SuppressibleRunEventType   `json:"excludedEphemeralEvents,omitempty"`
}

//flame:enum
type SuppressibleRunEventType string

const (
	SuppressibleRunSegmentProgress SuppressibleRunEventType = SuppressibleRunEventType(StreamSegmentProgress)
	SuppressibleRunItemDelta       SuppressibleRunEventType = SuppressibleRunEventType(StreamItemDelta)
)

type FeaturePreference struct {
	Enabled bool `json:"enabled"`
}

type ServerCapabilities struct {
	RunEvents        []StreamEventType            `json:"runEvents"`
	RuntimeTopics    []RuntimeTopic               `json:"runtimeTopics"`
	StreamingMethods []string                     `json:"streamingMethods"`
	Features         map[string]FeatureCapability `json:"features"`
	Limits           RuntimeLimits                `json:"limits"`
}

type FeatureCapability struct {
	Enabled       bool `json:"enabled"`
	ClientOptIn   bool `json:"clientOptIn"`
	RequiredByRun bool `json:"requiredByRun"`
}

func MissingFeatureRequirements(
	advertised map[string]FeatureCapability,
	client *ClientCapabilities,
	required ...string,
) []CapabilityRequirement {
	seen := make(map[string]bool, len(required))
	var missing []CapabilityRequirement
	for _, key := range required {
		if seen[key] {
			continue
		}
		seen[key] = true

		server := advertised[key]
		available := server.Enabled
		if available && server.ClientOptIn {
			available = client != nil && client.Features[key].Enabled
		}
		if !available {
			missing = append(missing, CapabilityRequirement{
				Type: RequirementFeature, Name: key,
			})
		}
	}
	return missing
}

type RuntimeLimits struct {
	MaxConcurrentRuns *int `json:"maxConcurrentRuns,omitzero"`
	// A client must not invent this window: retrying after it expires may execute the
	// command again.
	Idempotency              IdempotencyLimits             `json:"idempotency"`
	RunReplay                RunReplayLimits               `json:"runReplay"`
	MCPAuthorizationAttempts MCPAuthorizationAttemptLimits `json:"mcpAuthorizationAttempts"`
	RuntimeSubscription      SubscriptionLimits            `json:"runtimeSubscription"`
}

type IdempotencyLimits struct {
	RetentionSeconds int    `json:"retentionSeconds"`
	Namespace        string `json:"namespace"`
}

type MCPAuthorizationAttemptLimits struct {
	RetentionSeconds int `json:"retentionSeconds"`
}

//flame:enum
type RunReplayScope string

const (
	ReplayScopeRuntimeInstanceRootSegment RunReplayScope = "runtimeInstanceRootSegment"
)

type RunReplayLimits struct {
	Scope     RunReplayScope `json:"scope"`
	MaxEvents int            `json:"maxEvents"`
	MaxBytes  int            `json:"maxBytes"`
}

type SubscriptionLimits struct {
	MaxTopics           int   `json:"maxTopics"`
	MaxWatches          int   `json:"maxWatches"`
	MaxPaths            int   `json:"maxPaths"`
	MaxDirectoryEntries int   `json:"maxDirectoryEntries"`
	MaxFileBytes        int64 `json:"maxFileBytes"`
}
