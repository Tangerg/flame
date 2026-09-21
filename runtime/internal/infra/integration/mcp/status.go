package mcp

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured. Callers can distinguish it from a configured server whose
// connection attempt failed.
var ErrUnknownServer = errors.New("mcp: unknown server")

// ErrConnectionsClosed reports an operation attempted after the connection
// registry began shutting down. Shutdown is a terminal state: callers must build a
// new registry instead of reviving sessions behind the component owner's back.
var ErrConnectionsClosed = errors.New("mcp: connections closed")

// dialStatus maps a dial error to the connection status: an
// auth-distinguishable failure becomes "needsAuth" (so the client can prompt
// for credentials), otherwise "failed".
func dialStatus(err error) mcpserver.ConnectionState {
	if errors.Is(err, mcpserver.ErrAuthorizationRequired) {
		return mcpserver.ConnectionNeedsAuth
	}
	return mcpserver.ConnectionFailed
}
