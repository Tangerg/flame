package runtimeadapter

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/commandreplay"
	"github.com/Tangerg/flame/cli/internal/runtimeprofile"
)

func projectRuntimeProfile(
	discovery *protocol.DiscoverResponse,
	client *protocol.ClientCapabilities,
) (runtimeprofile.Profile, error) {
	if discovery == nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: discovery is nil")
	}
	runConcurrency, err := projectRunConcurrency(discovery.Capabilities.Limits.MaxConcurrentRuns)
	if err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: %w", err)
	}
	commandReplay, err := projectCommandReplay(discovery.Capabilities.Limits.Idempotency)
	if err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: %w", err)
	}
	profile := runtimeprofile.Profile{
		Protocol: runtimeprofile.Protocol{Version: discovery.ProtocolVersion},
		Server: runtimeprofile.Server{
			Name: discovery.ServerInfo.Name, Version: discovery.ServerInfo.Version,
			DefaultWorkspace: discovery.ServerInfo.DefaultWorkspace.Path, Home: discovery.ServerInfo.Home,
		},
		RunEvents:        make([]string, 0, len(discovery.Capabilities.RunEvents)),
		RuntimeTopics:    make([]string, 0, len(discovery.Capabilities.RuntimeTopics)),
		StreamingMethods: append([]string(nil), discovery.Capabilities.StreamingMethods...),
		Features:         make(map[runtimeprofile.FeatureName]runtimeprofile.Feature, len(discovery.Capabilities.Features)),
		Limits: runtimeprofile.Limits{
			RunConcurrency: runConcurrency,
			CommandReplay:  commandReplay,
			RunReplay: runtimeprofile.ReplayLimits{
				Scope:     string(discovery.Capabilities.Limits.RunReplay.Scope),
				MaxEvents: discovery.Capabilities.Limits.RunReplay.MaxEvents,
				MaxBytes:  discovery.Capabilities.Limits.RunReplay.MaxBytes,
			},
			MCPAuthorizationRetentionSeconds: discovery.Capabilities.Limits.MCPAuthorizationAttempts.RetentionSeconds,
			RuntimeSubscription: runtimeprofile.SubscriptionLimits{
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
		profile.Features[runtimeprofile.FeatureName(name)] = runtimeprofile.Feature{
			Enabled:     feature.Enabled,
			ClientOptIn: feature.ClientOptIn, ClientRequested: requested,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	if err := profile.Validate(); err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("project runtime profile: %w", err)
	}
	return profile, nil
}

func projectCommandReplay(limits protocol.IdempotencyLimits) (commandreplay.Capability, error) {
	if limits.RetentionSeconds <= 0 ||
		int64(limits.RetentionSeconds) > int64((time.Duration(1<<63-1))/time.Second) {
		return commandreplay.Capability{}, errors.New("command replay retention is outside the positive duration range")
	}
	capability, err := commandreplay.NewCapability(
		limits.Namespace, time.Duration(limits.RetentionSeconds)*time.Second,
	)
	if err != nil {
		return commandreplay.Capability{}, fmt.Errorf("command replay: %w", err)
	}
	return capability, nil
}

func projectRunConcurrency(maximum *int) (runtimeprofile.RunConcurrencyLimit, error) {
	if maximum == nil {
		return runtimeprofile.UnboundedRunConcurrencyLimit(), nil
	}
	limit, err := runtimeprofile.NewBoundedRunConcurrencyLimit(*maximum)
	if err != nil {
		return runtimeprofile.RunConcurrencyLimit{}, fmt.Errorf("run concurrency: %w", err)
	}
	return limit, nil
}
