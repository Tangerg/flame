package mcpserver

import (
	"errors"
	"fmt"
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
type ConnectionStatus struct {
	Name      ServerName
	State     ConnectionState
	ToolCount int
}

// ErrUnknownServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownServer = errors.New("mcp: unknown server")

// AdvertisedTool is one tool advertised by a connected MCP server.
type AdvertisedTool struct {
	Server      ServerName
	Name        RemoteToolName
	Description string
	InputSchema InputSchema
}

// Validate rechecks one complete live tool descriptor after it crosses the
// catalog port. The connection adapter validates at discovery time; the
// Application boundary independently protects alternate implementations and
// retained projections before exposing them to management or execution.
func (a AdvertisedTool) Validate() error {
	if err := a.Server.Validate(); err != nil {
		return fmt.Errorf("%w: server identity: %w", ErrInvalidRemoteToolCatalog, err)
	}
	if err := a.Name.Validate(); err != nil {
		return fmt.Errorf("%w: tool identity: %w", ErrInvalidRemoteToolCatalog, err)
	}
	if err := ValidateRemoteToolDescription(a.Description); err != nil {
		return err
	}
	if err := a.InputSchema.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRemoteToolCatalog, err)
	}
	return nil
}
