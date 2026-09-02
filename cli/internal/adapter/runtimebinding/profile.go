package runtimebinding

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func projectRuntimeProfile(
	discovery *protocol.DiscoverResponse,
	client *protocol.ClientCapabilities,
) (Profile, error) {
	if err := validateDiscovery(discovery); err != nil {
		return Profile{}, fmt.Errorf("project runtime profile: %w", err)
	}
	runConcurrency, err := projectRunConcurrency(discovery.Capabilities.Limits.MaxConcurrentRuns)
	if err != nil {
		return Profile{}, runtimeContractViolation("project runtime profile: %v", err)
	}
	commandReplay, err := projectCommandReplay(discovery.Capabilities.Limits.Idempotency)
	if err != nil {
		return Profile{}, runtimeContractViolation("project runtime profile: %v", err)
	}
	profile := Profile{
		Protocol: Protocol{Version: discovery.ProtocolVersion},
		Server: Server{
			Name: discovery.ServerInfo.Name, Version: discovery.ServerInfo.Version,
			DefaultWorkspace: discovery.ServerInfo.DefaultWorkspace.Path, Home: discovery.ServerInfo.Home,
		},
		RunEvents:        make([]string, 0, len(discovery.Capabilities.RunEvents)),
		RuntimeTopics:    make([]string, 0, len(discovery.Capabilities.RuntimeTopics)),
		StreamingMethods: append([]string(nil), discovery.Capabilities.StreamingMethods...),
		Features:         make(map[string]Feature, len(discovery.Capabilities.Features)),
		Limits: Limits{
			RunConcurrency: runConcurrency,
			CommandReplay:  commandReplay,
			RunReplay: ReplayLimits{
				Scope:     string(discovery.Capabilities.Limits.RunReplay.Scope),
				MaxEvents: discovery.Capabilities.Limits.RunReplay.MaxEvents,
				MaxBytes:  discovery.Capabilities.Limits.RunReplay.MaxBytes,
			},
			MCPAuthorizationRetentionSeconds: discovery.Capabilities.Limits.MCPAuthorizationAttempts.RetentionSeconds,
			RuntimeSubscription: SubscriptionLimits{
				MaxTopics:  discovery.Capabilities.Limits.RuntimeSubscription.MaxTopics,
				MaxWatches: discovery.Capabilities.Limits.RuntimeSubscription.MaxWatches,
			},
		},
	}
	for _, eventType := range discovery.Capabilities.RunEvents {
		profile.RunEvents = append(profile.RunEvents, string(eventType))
	}
	for _, topic := range discovery.Capabilities.RuntimeTopics {
		profile.RuntimeTopics = append(profile.RuntimeTopics, string(topic))
	}
	for name, feature := range discovery.Capabilities.Features {
		requested := client != nil && client.Features[name].Enabled
		profile.Features[name] = Feature{
			Enabled:     feature.Enabled,
			ClientOptIn: feature.ClientOptIn, ClientRequested: requested,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, runtimeContractViolation("project runtime profile: %v", err)
	}
	return profile, nil
}

func projectCommandReplay(limits protocol.IdempotencyLimits) (commandreplay.Capability, error) {
	capability, err := commandreplay.NewCapability(
		limits.Namespace, time.Duration(limits.RetentionSeconds)*time.Second,
	)
	if err != nil {
		return commandreplay.Capability{}, fmt.Errorf("command replay: %w", err)
	}
	return capability, nil
}

func projectRunConcurrency(maximum *int) (RunConcurrencyLimit, error) {
	if maximum == nil {
		return UnboundedRunConcurrencyLimit(), nil
	}
	limit, err := NewBoundedRunConcurrencyLimit(*maximum)
	if err != nil {
		return RunConcurrencyLimit{}, fmt.Errorf("run concurrency: %w", err)
	}
	return limit, nil
}
