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

var (
	// ErrUnknownServer is returned when a live MCP operation addresses a server
	// that was never configured.
	ErrUnknownServer = errors.New("mcp: unknown server")
	// ErrInvalidConnectionStatus reports a contradictory live status projection.
	ErrInvalidConnectionStatus = errors.New("mcp: invalid connection status")
)

func (s ConnectionState) Validate() error {
	switch s {
	case ConnectionConnecting, ConnectionConnected, ConnectionFailed, ConnectionNeedsAuth:
		return nil
	default:
		return fmt.Errorf("%w: unknown state %q", ErrInvalidConnectionStatus, s)
	}
}

// Validate protects the complete live projection. ToolCount belongs only to a
// connected server and shares the remote catalog's per-server ceiling.
func (s ConnectionStatus) Validate() error {
	if err := s.Name.Validate(); err != nil {
		return fmt.Errorf("%w: server identity: %w", ErrInvalidConnectionStatus, err)
	}
	if err := s.State.Validate(); err != nil {
		return err
	}
	if err := ValidateRemoteToolCount(s.ToolCount); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConnectionStatus, err)
	}
	if s.State != ConnectionConnected && s.ToolCount != 0 {
		return fmt.Errorf("%w: state %q carries tool count %d", ErrInvalidConnectionStatus, s.State, s.ToolCount)
	}
	return nil
}

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
