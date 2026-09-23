package mcpserver

import (
	"errors"

	"github.com/Tangerg/scope/core/chat"
)

// ConnectionState is the lifecycle state of a configured MCP connection.
// Keeping this vocabulary canonical prevents subtly different values for the
// same user-visible fact.
type ConnectionState string

const (
	ConnectionConnecting ConnectionState = "connecting"
	ConnectionConnected  ConnectionState = "connected"
	ConnectionFailed     ConnectionState = "failed"
	ConnectionNeedsAuth  ConnectionState = "needsAuth"
)

// ConnectionStatus is the safe, per-server live projection exposed by the MCP
// control plane. Connection failures stay in the operation and observability
// paths; a status is deliberately not an error transport.
//
// The connection pool owns this projection's invariants: it clears the tool set
// on every transition away from [ConnectionConnected], so a tool count outside
// that state is unrepresentable at the source rather than rejected downstream.
type ConnectionStatus struct {
	Name      ServerName
	State     ConnectionState
	ToolCount int
}

// ErrUnknownServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownServer = errors.New("mcp: unknown server")

var ErrAuthorizationRequired = errors.New("mcp: authorization required")

// AdvertisedTool projects an admitted Scope definition with its original MCP
// identity. The connection adapter transfers ownership of this snapshot.
type AdvertisedTool struct {
	Server     ServerName
	Name       RemoteToolName
	Definition chat.ToolDefinition
}
