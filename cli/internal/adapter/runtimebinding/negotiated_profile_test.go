package runtimebinding

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestProfileOwnsCapabilityCollectionsAndAnswersGates(t *testing.T) {
	t.Parallel()

	profile := validProfile(t)
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	if !profile.Supports(protocol.FeatureMCP) || profile.Supports(protocol.FeatureSchedules) || !profile.SupportsRuntimeTopic("files.changed") {
		t.Fatalf("profile gates = %+v", profile)
	}
	if names := profile.AvailableFeatureNames(); len(names) != 1 || names[0] != "mcp" {
		t.Fatalf("available features = %v", names)
	}
	clone := profile.Clone()
	clone.RunEvents[0] = "mutated"
	clone.RuntimeTopics[0] = "mutated"
	clone.StreamingMethods[0] = "mutated"
	clone.Features["mcp"] = Feature{}
	if profile.RunEvents[0] == "mutated" || profile.RuntimeTopics[0] == "mutated" ||
		profile.StreamingMethods[0] == "mutated" || !profile.Features["mcp"].Enabled {
		t.Fatal("profile clone retained caller-owned collections")
	}
}

func TestProfileRequiresClientAgreementForOptInFeatures(t *testing.T) {
	t.Parallel()

	profile := validProfile(t)
	profile.Features["subagents"] = Feature{Enabled: true, ClientOptIn: true}
	if profile.Supports(protocol.FeatureSubagents) {
		t.Fatal("server support bypassed the client opt-in requirement")
	}
	feature := profile.Features["subagents"]
	feature.ClientRequested = true
	profile.Features["subagents"] = feature
	if !profile.Supports(protocol.FeatureSubagents) {
		t.Fatal("negotiated opt-in feature was unavailable")
	}
}

func TestProfileRejectsIncompleteIdentityCapabilitiesAndLimits(t *testing.T) {
	t.Parallel()

	tests := []Profile{
		{},
		func() Profile {
			value := validProfile(t)
			value.RunEvents = append(value.RunEvents, value.RunEvents[0])
			return value
		}(),
		func() Profile { value := validProfile(t); value.Limits.RunReplay.MaxBytes = 0; return value }(),
		func() Profile { value := validProfile(t); value.Limits.RunReplay.Scope = "future"; return value }(),
		func() Profile { value := validProfile(t); value.Limits.RuntimeSubscription.MaxTopics = 0; return value }(),
	}
	for _, profile := range tests {
		if err := profile.Validate(); err == nil {
			t.Fatalf("Validate accepted invalid profile: %+v", profile)
		}
	}
}

func TestRunConcurrencyRequiresExplicitValidPolicyAndRoundTripsJSON(t *testing.T) {
	t.Parallel()

	if err := (RunConcurrencyLimit{}).Validate(); err == nil {
		t.Fatal("zero RunConcurrencyLimit was valid")
	}
	if _, err := NewBoundedRunConcurrencyLimit(0); err == nil {
		t.Fatal("bounded zero Run concurrency was valid")
	}
	for _, limit := range []RunConcurrencyLimit{
		UnboundedRunConcurrencyLimit(),
		boundedRunConcurrency(t, 4),
	} {
		encoded, err := json.Marshal(limit)
		if err != nil {
			t.Fatal(err)
		}
		var decoded RunConcurrencyLimit
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded != limit {
			t.Fatalf("round trip = %+v, want %+v", decoded, limit)
		}
	}
	for _, invalid := range []string{
		`{"type":"unbounded","maximum":1}`,
		`{"type":"bounded"}`,
		`{"type":"bounded","maximum":0}`,
		`{"type":"future"}`,
		`{"type":"unbounded","extra":true}`,
		`{"type":"unbounded"} {"type":"unbounded"}`,
	} {
		var decoded RunConcurrencyLimit
		if err := json.Unmarshal([]byte(invalid), &decoded); err == nil {
			t.Fatalf("decoded invalid Run concurrency %s", invalid)
		}
	}
}

func validProfile(t *testing.T) Profile {
	t.Helper()
	return Profile{
		Protocol: Protocol{Version: "2.0"},
		Server: Server{
			Name: "flame-runtime", Version: "dev", DefaultWorkspace: "/workspace", Home: "/home/test",
		},
		RunEvents:        []string{"segment.started"},
		RuntimeTopics:    []string{"files.changed"},
		StreamingMethods: []string{"runs.start"},
		Features: map[string]Feature{
			protocol.FeatureMCP:       {Enabled: true},
			protocol.FeatureSchedules: {},
		},
		Limits: Limits{
			RunConcurrency:                   boundedRunConcurrency(t, 4),
			CommandReplay:                    commandReplayCapability(t, "idp_test", 10*time.Minute),
			RunReplay:                        protocol.RunReplayLimits{Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1024, MaxBytes: 1 << 20},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              protocol.SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
		},
	}
}

func commandReplayCapability(t *testing.T, namespace string, retention time.Duration) commandreplay.Capability {
	t.Helper()
	capability, err := commandreplay.NewCapability(namespace, retention)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func boundedRunConcurrency(t *testing.T, maximum int) RunConcurrencyLimit {
	t.Helper()
	limit, err := NewBoundedRunConcurrencyLimit(maximum)
	if err != nil {
		t.Fatal(err)
	}
	return limit
}
