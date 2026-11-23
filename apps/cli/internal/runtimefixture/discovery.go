package runtimefixture

import (
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	fixtureConcurrentRuns              = 4
	fixtureIdempotencyRetentionSeconds = 600
	fixtureReplayEvents                = 1024
	fixtureWatchPaths                  = 64
	fixtureWatchDirectoryEntries       = 1024
)

func Discovery() *protocol.DiscoverResponse {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.Version,
		ServerInfo: protocol.ServerInfo{
			InstanceID: "runtime_01234567-89ab-4cde-8f01-23456789abcd", Name: "flame-runtime", Version: "1.2.3",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home/test",
		},
		Capabilities: protocol.ServerCapabilities{
			RunEvents:        []protocol.StreamEventType{protocol.StreamSegmentStarted, protocol.StreamSegmentProgress, protocol.StreamSegmentFinished, protocol.StreamItemStarted, protocol.StreamItemDelta, protocol.StreamItemCompleted, protocol.StreamPlanUpdated},
			RuntimeTopics:    protocol.RuntimeTopics(),
			StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe"},
			Features:         map[string]protocol.FeatureCapability{},
			Limits: protocol.RuntimeLimits{
				MaxConcurrentRuns:        new(fixtureConcurrentRuns),
				Idempotency:              protocol.IdempotencyLimits{RetentionSeconds: fixtureIdempotencyRetentionSeconds, Namespace: "idp_0123456789abcdef0123456789abcdef"},
				RunReplay:                protocol.RunReplayLimits{Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: fixtureReplayEvents, MaxBytes: filesystem.MiB},
				MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: mcpconnector.AttemptRetentionSeconds},
				RuntimeSubscription:      protocol.SubscriptionLimits{MaxTopics: protocol.MaxSubscriptionTopics, MaxWatches: protocol.MaxSubscriptionWatches, MaxPaths: fixtureWatchPaths, MaxDirectoryEntries: fixtureWatchDirectoryEntries, MaxFileBytes: filesystem.MiB},
			},
		},
	}
}

func CompleteDiscovery() (*protocol.DiscoverResponse, *protocol.ClientCapabilities) {
	discovery := Discovery()
	client := &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{}}
	for _, feature := range protocol.Features() {
		discovery.Capabilities.Features[feature.Key] = protocol.FeatureCapability{
			Enabled: true, ClientOptIn: feature.ClientOptIn, RequiredByRun: feature.RequiredByRun,
		}
		if feature.ClientOptIn {
			client.Features[feature.Key] = protocol.FeaturePreference{Enabled: true}
		}
	}
	return discovery, client
}
