package runtimebinding

import (
	"errors"

	"slices"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestNegotiatedRunCapabilitiesMatchProjectionBoundary(t *testing.T) {
	t.Parallel()
	meta := requestMeta("test")
	if meta.ClientCapabilities == nil {
		t.Fatal("request metadata omitted client capabilities")
	}
	if preference := meta.ClientCapabilities.Features[protocol.FeatureChildRuns]; !preference.Enabled {
		t.Fatal("request metadata does not negotiate the supported subagent stream profile")
	}
	wantInterrupts := supportedInterruptTypes()
	if !slices.Equal(meta.ClientCapabilities.InterruptTypes, wantInterrupts) {
		t.Fatalf("negotiated interrupts = %v, projection supports %v", meta.ClientCapabilities.InterruptTypes, wantInterrupts)
	}
	if len(meta.ClientCapabilities.ExcludedEphemeralEvents) != 0 {
		t.Fatalf("client unexpectedly suppresses runtime events: %v", meta.ClientCapabilities.ExcludedEphemeralEvents)
	}
	for _, eventType := range requiredRunEventTypes() {
		if !slices.Contains(recognizedRunEventTypes(), eventType) {
			t.Fatalf("required run event %q has no projection policy", eventType)
		}
	}
}

func TestDiscoveryRejectsUnprojectedStreamAndChangeCapabilities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*protocol.DiscoverResponse)
	}{
		{
			name: "run event",
			mutate: func(discovery *protocol.DiscoverResponse) {
				discovery.Capabilities.RunEvents = append(discovery.Capabilities.RunEvents, "vendor.authoritative")
			},
		},
		{
			name: "runtime topic",
			mutate: func(discovery *protocol.DiscoverResponse) {
				discovery.Capabilities.RuntimeTopics = append(discovery.Capabilities.RuntimeTopics, "indexes.changed")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			discovery := compatibleDiscovery()
			test.mutate(discovery)
			if err := validateDiscovery(discovery); !errors.Is(err, conversation.ErrIncompatibleRuntime) {
				t.Fatalf("validateDiscovery = %v, want ErrIncompatibleRuntime", err)
			}
		})
	}
}

func TestDiscoveryAcceptsRuntimeWithoutOptionalPlanCapability(t *testing.T) {
	t.Parallel()
	discovery := compatibleDiscovery()
	feature := discovery.Capabilities.Features[protocol.FeaturePlan]
	feature.Enabled = false
	discovery.Capabilities.Features[protocol.FeaturePlan] = feature
	if err := validateDiscovery(discovery); err != nil {
		t.Fatalf("validateDiscovery rejected a runtime without optional plan support: %v", err)
	}
}

func compatibleDiscovery() *protocol.DiscoverResponse {
	maxConcurrentRuns := 4
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.Version,
		ServerInfo: protocol.ServerInfo{
			Name: "flame-runtime", Version: "test",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home/test",
		},
		Capabilities: protocol.ServerCapabilities{
			RunEvents:        recognizedRunEventTypes(),
			RuntimeTopics:    protocol.RuntimeTopics(),
			StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe"},
			Features:         map[string]protocol.FeatureCapability{protocol.FeaturePlan: {Enabled: true}},
			Limits: protocol.RuntimeLimits{
				MaxConcurrentRuns: &maxConcurrentRuns,
				Idempotency:       protocol.IdempotencyLimits{RetentionSeconds: 600, Namespace: compatibleReplayNamespace},
				RunReplay: protocol.RunReplayLimits{
					Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1024, MaxBytes: 1 << 20,
				},
				MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 600},
				RuntimeSubscription:      protocol.SubscriptionLimits{MaxTopics: 32, MaxWatches: 32},
			},
		},
	}
}

const compatibleReplayNamespace = "idp_0123456789abcdef0123456789abcdef"
