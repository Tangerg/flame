package protocol

import (
	"maps"
	"slices"
)

// RequestMetaMember carries RequestMeta inside every request's params object.
const RequestMetaMember = "_meta"

type RequestMeta struct {
	ProtocolVersion    *string             `json:"protocolVersion,omitzero"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitzero"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitzero"`
}

func (m RequestMeta) Clone() RequestMeta {
	if m.ProtocolVersion != nil {
		m.ProtocolVersion = new(*m.ProtocolVersion)
	}
	if m.ClientInfo != nil {
		m.ClientInfo = new(*m.ClientInfo)
	}
	if m.ClientCapabilities != nil {
		capabilities := *m.ClientCapabilities
		capabilities.Features = maps.Clone(capabilities.Features)
		capabilities.InterruptTypes = slices.Clone(capabilities.InterruptTypes)
		capabilities.ExcludedEphemeralEvents = slices.Clone(capabilities.ExcludedEphemeralEvents)
		m.ClientCapabilities = &capabilities
	}
	return m
}
