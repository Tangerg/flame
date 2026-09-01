package runtimebinding

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func TestRuntimeProfileProjectionPreservesCompleteDiscovery(t *testing.T) {
	t.Parallel()

	discovery := compatibleDiscovery()
	discovery.Capabilities.Features = map[string]protocol.FeatureCapability{
		protocol.FeatureMCP: {
			Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true,
		},
	}
	profile, err := projectRuntimeProfile(discovery, &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{
		protocol.FeatureMCP: {Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Server.Name != "flame-runtime" || profile.Server.DefaultWorkspace != "/workspace" ||
		profile.Protocol.Version != protocol.ProtocolVersion ||
		len(profile.RunEvents) != len(discovery.Capabilities.RunEvents) ||
		len(profile.RuntimeTopics) != len(discovery.Capabilities.RuntimeTopics) ||
		len(profile.StreamingMethods) != len(discovery.Capabilities.StreamingMethods) {
		t.Fatalf("runtime profile = %+v", profile)
	}
	feature := profile.Features[protocol.FeatureMCP]
	if !feature.Enabled || !feature.ClientOptIn ||
		!feature.ClientRequested || !feature.RequiredByRunProtocol || !feature.Available() {
		t.Fatalf("MCP profile = %+v", feature)
	}
	limits := profile.Limits
	maximum, bounded := limits.RunConcurrency.Maximum()
	if !bounded || maximum != 4 || limits.CommandReplay.Retention() != 10*time.Minute ||
		limits.CommandReplay.Namespace() != "idp_test" ||
		limits.RunReplay.MaxEvents != 1024 || limits.RunReplay.MaxBytes != 1<<20 ||
		limits.MCPAuthorizationRetentionSeconds != 600 ||
		limits.RuntimeSubscription.MaxTopics != 32 || limits.RuntimeSubscription.MaxWatches != 32 {
		t.Fatalf("runtime limits = %+v", limits)
	}
}

func testCommandReplay(t *testing.T, namespace string) commandreplay.Capability {
	t.Helper()
	capability, err := commandreplay.NewCapability(namespace, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func TestRuntimeProfileProjectionDistinguishesUnboundedFromInvalidConcurrency(t *testing.T) {
	t.Parallel()

	unboundedDiscovery := compatibleDiscovery()
	unboundedDiscovery.Capabilities.Limits.MaxConcurrentRuns = nil
	unbounded, err := projectRuntimeProfile(unboundedDiscovery, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, bounded := unbounded.Limits.RunConcurrency.Maximum(); bounded {
		t.Fatal("absent runtime concurrency cap was projected as bounded")
	}

	zero := 0
	invalidDiscovery := compatibleDiscovery()
	invalidDiscovery.Capabilities.Limits.MaxConcurrentRuns = &zero
	if _, err := projectRuntimeProfile(invalidDiscovery, nil); err == nil {
		t.Fatal("explicit zero runtime concurrency cap was accepted")
	}
}

func TestConnectionReturnsOwnedProfilesWithoutForkingCapabilityPolicy(t *testing.T) {
	t.Parallel()

	profile, err := projectRuntimeProfile(compatibleDiscovery(), nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &Connection{profile: profile}
	first := runtime.Profile()
	second := runtime.Profile()
	first.RuntimeTopics[0] = "mutated"
	if runtime.profile.RuntimeTopics[0] == "mutated" || second.RuntimeTopics[0] == "mutated" {
		t.Fatal("connection profile mutation crossed an ownership boundary")
	}
}

func TestConnectionNeverInfersProfileValidityFromServerName(t *testing.T) {
	t.Parallel()

	runtime := &Connection{}
	if err := runtime.Profile().Validate(); err == nil {
		t.Fatal("invalid connection profile was mistaken for negotiated discovery")
	}
}
