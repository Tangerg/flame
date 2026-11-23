package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (s *Handler) Discover(_ context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.Version,
		ServerInfo:      s.serverInfo,
		Capabilities:    s.capabilities(),
	}, nil
}
