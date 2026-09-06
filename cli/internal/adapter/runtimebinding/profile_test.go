package runtimebinding

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestRuntimeProfilePreservesDiscoveryAndNegotiatesWithProtocolRules(t *testing.T) {
	t.Parallel()
	discovery := compatibleDiscovery()
	discovery.Capabilities.Features = map[string]protocol.FeatureCapability{
		protocol.FeatureMCP:       {Enabled: true},
		protocol.FeatureSubagents: {Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true},
		protocol.FeatureSchedules: {},
	}
	for _, enabled := range []bool{false, true} {
		client := &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{
			protocol.FeatureSubagents: {Enabled: enabled},
		}}
		profile := requireProfile(t, discovery, client)
		if !reflect.DeepEqual(profile.Discovery(), *discovery) || !reflect.DeepEqual(profile.ClientCapabilities(), client) {
			t.Fatal("profile changed discovery or client capability values")
		}
		if !profile.Supports(protocol.FeatureMCP) || profile.Supports(protocol.FeatureSchedules) ||
			profile.Supports(protocol.FeatureSubagents) != enabled || !profile.SupportsRuntimeTopic(protocol.TopicFilesChanged) {
			t.Fatal("profile did not preserve the protocol capability agreement")
		}
		wantNames := []string{protocol.FeatureMCP}
		if enabled {
			wantNames = append(wantNames, protocol.FeatureSubagents)
		}
		if !reflect.DeepEqual(profile.AvailableFeatureNames(), wantNames) {
			t.Fatalf("available features = %v, want %v", profile.AvailableFeatureNames(), wantNames)
		}
	}
}

func TestRuntimeProfileProtectsRetainedProtocolValues(t *testing.T) {
	t.Parallel()
	discovery := compatibleDiscovery()
	client := requestMeta("test").ClientCapabilities
	client.ExcludedEphemeralEvents = []protocol.SuppressibleRunEventType{protocol.SuppressibleRunItemDelta}
	profile := requireProfile(t, discovery, client)
	runtime := &Connection{profile: profile}
	wantDiscovery, wantClient := profile.Discovery(), profile.ClientCapabilities()
	mutate := func(d *protocol.DiscoverResponse, c *protocol.ClientCapabilities) {
		d.Capabilities.RunEvents[0] = "mutated"
		d.Capabilities.RuntimeTopics[0] = "mutated"
		d.Capabilities.StreamingMethods[0] = "mutated"
		d.Capabilities.Features[protocol.FeaturePlan] = protocol.FeatureCapability{}
		*d.Capabilities.Limits.MaxConcurrentRuns = 99
		c.Features[protocol.FeatureSubagents] = protocol.FeaturePreference{}
		c.InterruptTypes[0] = "mutated"
		c.ExcludedEphemeralEvents[0] = protocol.SuppressibleRunSegmentProgress
	}
	mutate(discovery, client)
	first := runtime.Profile()
	readDiscovery, readClient := first.Discovery(), first.ClientCapabilities()
	mutate(&readDiscovery, readClient)
	for _, got := range []Profile{profile, first, runtime.Profile()} {
		if !reflect.DeepEqual(got.Discovery(), wantDiscovery) || !reflect.DeepEqual(got.ClientCapabilities(), wantClient) {
			t.Fatal("mutating a source or returned protocol value changed negotiated facts")
		}
	}
}

func TestRuntimeProfileRejectsInvalidDiscoveryAtConstruction(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		invalidate func(*protocol.DiscoverResponse)
	}{
		{"protocol", func(d *protocol.DiscoverResponse) { d.ProtocolVersion = "" }},
		{"identity", func(d *protocol.DiscoverResponse) { d.ServerInfo.Name = "" }},
		{"duplicate events", func(d *protocol.DiscoverResponse) {
			d.Capabilities.RunEvents = append(d.Capabilities.RunEvents, d.Capabilities.RunEvents[0])
		}},
		{"zero concurrency", func(d *protocol.DiscoverResponse) { d.Capabilities.Limits.MaxConcurrentRuns = new(0) }},
		{"zero replay bytes", func(d *protocol.DiscoverResponse) { d.Capabilities.Limits.RunReplay.MaxBytes = 0 }},
		{"unknown replay scope", func(d *protocol.DiscoverResponse) { d.Capabilities.Limits.RunReplay.Scope = "future" }},
		{"zero subscription topics", func(d *protocol.DiscoverResponse) { d.Capabilities.Limits.RuntimeSubscription.MaxTopics = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			discovery := compatibleDiscovery()
			test.invalidate(discovery)
			if _, err := NewProfile(discovery, nil); !errors.Is(err, agent.ErrIncompatibleRuntime) {
				t.Fatalf("NewProfile = %v, want incompatible runtime", err)
			}
		})
	}
	if _, err := NewProfile(nil, nil); err == nil {
		t.Fatal("nil discovery was accepted")
	}
	if err := (Profile{}).Validate(); err == nil {
		t.Fatal("unnegotiated profile was valid")
	}
}

func TestRuntimeProfilePreservesUnboundedConcurrency(t *testing.T) {
	t.Parallel()
	discovery := compatibleDiscovery()
	discovery.Capabilities.Limits.MaxConcurrentRuns = nil
	if requireProfile(t, discovery, nil).Discovery().Capabilities.Limits.MaxConcurrentRuns != nil {
		t.Fatal("absent runtime concurrency cap became bounded")
	}
}

func TestRuntimeProfileRejectsRetentionThatWouldWrapToOneSecond(t *testing.T) {
	t.Parallel()
	// Multiplying this value by time.Second modulo 2^64 yields exactly one
	// second, so validation after conversion cannot detect the overflow.
	wrapsToOneSecond := int64(36_028_797_018_963_969)
	if int64(int(wrapsToOneSecond)) != wrapsToOneSecond {
		t.Skip("int width cannot represent an overflowing duration")
	}
	discovery := compatibleDiscovery()
	discovery.Capabilities.Limits.Idempotency.RetentionSeconds = int(wrapsToOneSecond)
	_, err := NewProfile(discovery, nil)
	if !errors.Is(err, agent.ErrIncompatibleRuntime) || !strings.Contains(err.Error(), "retentionSeconds") {
		t.Fatalf("NewProfile overflow error = %v", err)
	}
}

func requireProfile(t *testing.T, discovery *protocol.DiscoverResponse, client *protocol.ClientCapabilities) Profile {
	t.Helper()
	profile, err := NewProfile(discovery, client)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func profileWithFeatures(t *testing.T, features map[string]protocol.FeatureCapability) Profile {
	t.Helper()
	discovery := compatibleDiscovery()
	discovery.Capabilities.Features = features
	return requireProfile(t, discovery, requestMeta("test").ClientCapabilities)
}

func profileWithReplayNamespace(t *testing.T, namespace string) Profile {
	t.Helper()
	discovery := compatibleDiscovery()
	discovery.Capabilities.Limits.Idempotency.Namespace = namespace
	return requireProfile(t, discovery, nil)
}
