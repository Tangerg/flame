package runtimebinding

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type Profile struct {
	discovery protocol.DiscoverResponse
	client    *protocol.ClientCapabilities
}

func NewProfile(discovery *protocol.DiscoverResponse, client *protocol.ClientCapabilities) (Profile, error) {
	if err := validateDiscovery(discovery); err != nil {
		return Profile{}, fmt.Errorf("negotiate runtime profile: %w", err)
	}
	if client != nil {
		if err := protocol.ValidateWireTree(*client); err != nil {
			return Profile{}, fmt.Errorf("negotiate client capabilities: %w", err)
		}
	}
	return Profile{discovery: cloneDiscovery(*discovery), client: cloneClientCapabilities(client)}, nil
}

func (p Profile) Discovery() protocol.DiscoverResponse { return cloneDiscovery(p.discovery) }

func (p Profile) ClientCapabilities() *protocol.ClientCapabilities {
	return cloneClientCapabilities(p.client)
}

func (p Profile) IdempotencyLimits() protocol.IdempotencyLimits {
	return p.discovery.Capabilities.Limits.Idempotency
}

func (p Profile) Supports(feature string) bool {
	return len(protocol.MissingFeatureRequirements(p.discovery.Capabilities.Features, p.client, feature)) == 0
}

func (p Profile) supportsRuntimeTopic(topic protocol.RuntimeTopic) bool {
	return slices.Contains(p.discovery.Capabilities.RuntimeTopics, topic)
}

func (p Profile) AvailableFeatureNames() []string {
	var names []string
	for name := range p.discovery.Capabilities.Features {
		if p.Supports(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func cloneDiscovery(discovery protocol.DiscoverResponse) protocol.DiscoverResponse {
	capabilities := &discovery.Capabilities
	capabilities.RunEvents = slices.Clone(capabilities.RunEvents)
	capabilities.RuntimeTopics = slices.Clone(capabilities.RuntimeTopics)
	capabilities.StreamingMethods = slices.Clone(capabilities.StreamingMethods)
	capabilities.Features = maps.Clone(capabilities.Features)
	if maximum := capabilities.Limits.MaxConcurrentRuns; maximum != nil {
		capabilities.Limits.MaxConcurrentRuns = new(*maximum)
	}
	return discovery
}

func cloneClientCapabilities(client *protocol.ClientCapabilities) *protocol.ClientCapabilities {
	if client == nil {
		return nil
	}
	cloned := *client
	cloned.Features = maps.Clone(client.Features)
	cloned.InterruptTypes = slices.Clone(client.InterruptTypes)
	cloned.ExcludedEphemeralEvents = slices.Clone(client.ExcludedEphemeralEvents)
	return &cloned
}
