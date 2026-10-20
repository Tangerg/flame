package runtimefixture

import "github.com/Tangerg/flame/runtime/protocol"

// Discovery supplies a complete protocol response for consumer tests. Tests
// customize advertised facts before negotiating them through the real adapter.
func Discovery() *protocol.DiscoverResponse {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo: protocol.ServerInfo{
			InstanceID: "runtime_01234567-89ab-4cde-8f01-23456789abcd", Name: "flame-runtime", Version: "1.2.3",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home/test",
		},
		Capabilities: protocol.ServerCapabilities{
			RunEvents: []protocol.StreamEventType{
				protocol.StreamSegmentStarted, protocol.StreamSegmentProgress, protocol.StreamSegmentFinished,
				protocol.StreamItemStarted, protocol.StreamItemDelta, protocol.StreamItemCompleted, protocol.StreamPlanUpdated,
			},
			RuntimeTopics:    protocol.RuntimeTopics(),
			StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe"},
			Features:         map[string]protocol.FeatureCapability{},
			Limits: protocol.RuntimeLimits{
				MaxConcurrentRuns:        new(4),
				Idempotency:              protocol.IdempotencyLimits{RetentionSeconds: 600, Namespace: "idp_0123456789abcdef0123456789abcdef"},
				RunReplay:                protocol.RunReplayLimits{Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1024, MaxBytes: 1 << 20},
				MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 600},
				RuntimeSubscription:      protocol.SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
			},
		},
	}
}

// CompleteDiscovery advertises every published feature, and the client
// agreement that opts into each one requiring it, as a complete Runtime
// composition negotiated by the CLI would.
func CompleteDiscovery() (*protocol.DiscoverResponse, *protocol.ClientCapabilities) {
	discovery := Discovery()
	client := &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{}}
	for _, feature := range protocol.Features() {
		discovery.Capabilities.Features[feature.Key] = protocol.FeatureCapability{
			Enabled: true, ClientOptIn: feature.ClientOptIn, RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
		if feature.ClientOptIn {
			client.Features[feature.Key] = protocol.FeaturePreference{Enabled: true}
		}
	}
	return discovery, client
}
