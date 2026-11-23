package protocol

import (
	"maps"
	"slices"
)

// RequestMetaMember carries RequestMeta inside every request's params object.
const RequestMetaMember = "_meta"

type RequestMeta struct {
	ProtocolVersion    string              `json:"protocolVersion,omitempty"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitzero"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitzero"`
}

func (m RequestMeta) Clone() RequestMeta {
	if m.ClientInfo != nil {
		info := *m.ClientInfo
		m.ClientInfo = &info
	}
	if m.ClientCapabilities != nil {
		capabilities := ClientCapabilities{
			Features:                maps.Clone(m.ClientCapabilities.Features),
			InterruptTypes:          slices.Clone(m.ClientCapabilities.InterruptTypes),
			ExcludedEphemeralEvents: slices.Clone(m.ClientCapabilities.ExcludedEphemeralEvents),
		}
		m.ClientCapabilities = &capabilities
	}
	return m
}
