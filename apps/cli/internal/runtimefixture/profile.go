package runtimefixture

import (
	json "encoding/json/v2"
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// Profile supplies negotiated facts to consumer tests without opening a transport.
// Negotiation itself is exercised inside the Runtime binding adapter.
type Profile struct {
	discovery protocol.DiscoverResponse
	client    *protocol.ClientCapabilities
}

func NewProfile(discovery *protocol.DiscoverResponse, client *protocol.ClientCapabilities) (Profile, error) {
	if discovery == nil {
		return Profile{}, errors.New("test discovery is required")
	}
	if err := protocol.ValidateWireTree(*discovery); err != nil {
		return Profile{}, err
	}
	if client != nil {
		if err := protocol.ValidateWireTree(*client); err != nil {
			return Profile{}, err
		}
	}
	return Profile{discovery: copyProfileValue(*discovery), client: copyProfileValue(client)}, nil
}
func (p Profile) Validate() error {
	if p.discovery.ProtocolVersion == "" {
		return errors.New("runtime profile has not been negotiated")
	}
	return nil
}
func (p Profile) Discovery() protocol.DiscoverResponse             { return copyProfileValue(p.discovery) }
func (p Profile) ClientCapabilities() *protocol.ClientCapabilities { return copyProfileValue(p.client) }
func (p Profile) IdempotencyLimits() protocol.IdempotencyLimits {
	return p.discovery.Capabilities.Limits.Idempotency
}
func (p Profile) Supports(feature string) bool {
	return len(protocol.MissingFeatureRequirements(p.discovery.Capabilities.Features, p.client, feature)) == 0
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
func copyProfileValue[T any](value T) T {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		panic(err)
	}
	return result
}
