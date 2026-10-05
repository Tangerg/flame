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

// ConnectionFailure is the closed category of why a server is not connected
// while its state is [ConnectionFailed]. It is the safe projection of a cause:
// the cause itself can carry paths, endpoints or credentials and stays in the
// observability path, while the category reaches every status reader.
type ConnectionFailure string

const (
	// FailureUnavailableRelease: the owning installation's release bytes
	// cannot be verified, so no configuration exists to connect with.
	FailureUnavailableRelease ConnectionFailure = "unavailableRelease"
	// FailureUnavailableBackend: the release is intact but the server's
	// backend directory cannot be realized.
	FailureUnavailableBackend ConnectionFailure = "unavailableBackend"
	// FailureConfiguration: the configuration owner refused the connection or
	// could not be read, or its stored credentials could not be restored.
	FailureConfiguration ConnectionFailure = "configuration"
	// FailureConnection: the transport, process or MCP handshake failed.
	FailureConnection ConnectionFailure = "connection"
	// FailureToolDiscovery: a session was established but did not produce a
	// valid tool catalog.
	FailureToolDiscovery ConnectionFailure = "toolDiscovery"
	// FailureAuthorization: an interactive sign-in did not complete.
	FailureAuthorization ConnectionFailure = "authorization"
)

// ConnectionStatus is the safe, per-server live projection exposed by the MCP
// control plane. It carries the failure category, never the failure cause.
//
// The connection pool owns this projection's invariants: it clears the tool set
// on every transition away from [ConnectionConnected] and sets Failure exactly
// when entering [ConnectionFailed], so a tool count or failure outside its state
// is unrepresentable at the source rather than rejected downstream.
type ConnectionStatus struct {
	Server    ID
	State     ConnectionState
	Failure   ConnectionFailure
	ToolCount int
}

// ErrUnknownServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownServer = errors.New("mcp: unknown server")

var ErrAuthorizationRequired = errors.New("mcp: authorization required")

// AdvertisedTool projects an admitted Scope definition with its original MCP
// identity. The connection adapter transfers ownership of this snapshot.
type AdvertisedTool struct {
	Server     ID
	Name       RemoteToolName
	Definition chat.ToolDefinition
}
