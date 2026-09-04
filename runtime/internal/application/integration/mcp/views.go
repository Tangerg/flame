package mcp

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/secrets"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// SecretChangeKind is the application's exact secret-mutation vocabulary.
// Persistence never has to infer intent from an empty string.
type SecretChangeKind string

const (
	SecretSet   SecretChangeKind = "set"
	SecretClear SecretChangeKind = "clear"
)

// Valid reports whether kind names one exact secret mutation.
func (s SecretChangeKind) Valid() bool { return s == SecretSet || s == SecretClear }

// String returns the stable secret-mutation name.
func (s SecretChangeKind) String() string {
	if !s.Valid() {
		return "unknown"
	}
	return string(s)
}

// AuthorizationChange is a write-only bearer-token mutation.
type AuthorizationChange struct {
	Kind  SecretChangeKind
	Value string
}

// HeadersChange is a write-only full replacement for HTTP headers.
// Authorization remains the dedicated [AuthorizationChange] field.
type HeadersChange struct {
	Kind  SecretChangeKind
	Value map[string]string
}

// EnvironmentChange is a write-only full replacement for a stdio process's
// environment.
type EnvironmentChange struct {
	Kind  SecretChangeKind
	Value map[string]string
}

// ConnectionInput is a complete connection replacement. Transport-specific
// validation happens before it becomes the domain's flat persistence descriptor.
type ConnectionInput struct {
	Transport     mcpserver.Transport
	URL           string
	Authorization *AuthorizationChange
	Headers       *HeadersChange
	Command       string
	Args          []string
	Environment   *EnvironmentChange
	Dir           string
}

func (c ConnectionInput) clone() ConnectionInput {
	c.Authorization = clonePointer(c.Authorization)
	if c.Headers != nil {
		change := *c.Headers
		change.Value = maps.Clone(change.Value)
		c.Headers = &change
	}
	c.Args = slices.Clone(c.Args)
	if c.Environment != nil {
		change := *c.Environment
		change.Value = maps.Clone(change.Value)
		c.Environment = &change
	}
	return c
}

// ServerInput is a complete create/test candidate.
type ServerInput struct {
	Name             mcpserver.ServerName
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	HandshakeTimeout mcpserver.HandshakeTimeout
	ToolPolicy       mcpserver.ServerToolPolicy
}

func (s ServerInput) clone() ServerInput {
	s.Connection = s.Connection.clone()
	return s
}

// ServerPatch is an update command. nil preserves the current value; a
// present zero or empty collection clears it.
type ServerPatch struct {
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	HandshakeTimeout *mcpserver.HandshakeTimeout
	DisabledTools    *[]mcpserver.RemoteToolName
	AutoApproveTools *[]mcpserver.RemoteToolName
}

// Empty reports whether the update carries no mutation.
func (s ServerPatch) Empty() bool {
	return s.Enabled == nil && s.Description == nil && s.Connection == nil &&
		s.HandshakeTimeout == nil && s.DisabledTools == nil && s.AutoApproveTools == nil
}

func (s ServerPatch) clone() ServerPatch {
	s.Enabled = clonePointer(s.Enabled)
	s.Description = clonePointer(s.Description)
	if s.Connection != nil {
		connection := s.Connection.clone()
		s.Connection = &connection
	}
	s.HandshakeTimeout = clonePointer(s.HandshakeTimeout)
	if s.DisabledTools != nil {
		tools := slices.Clone(*s.DisabledTools)
		s.DisabledTools = &tools
	}
	if s.AutoApproveTools != nil {
		tools := slices.Clone(*s.AutoApproveTools)
		s.AutoApproveTools = &tools
	}
	return s
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// Connection is the safe application read model for a connection. Raw
// secret-bearing values never cross the application boundary.
type Connection struct {
	Transport           mcpserver.Transport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Dir                 string
}

// Server is the unified application read model: durable configuration and
// the current connection lifecycle are projected together.
type Server struct {
	Name             mcpserver.ServerName
	Description      string
	Connection       Connection
	HandshakeTimeout mcpserver.HandshakeTimeout
	ToolPolicy       mcpserver.ServerToolPolicy
	State            ServerState
}

// ServerState is the application's complete server lifecycle. Disabled is
// represented explicitly rather than as an absent or contradictory status.
type ServerState struct {
	Type      ServerStateType
	ToolCount *int
}

type ServerStateType string

const (
	ServerDisabled     ServerStateType = "disabled"
	ServerDisconnected ServerStateType = "disconnected"
	ServerConnecting   ServerStateType = "connecting"
	ServerConnected    ServerStateType = "connected"
	ServerFailed       ServerStateType = "failed"
	ServerNeedsAuth    ServerStateType = "needsAuth"
)

// Valid reports whether s belongs to the complete MCP server lifecycle.
func (s ServerStateType) Valid() bool {
	switch s {
	case ServerDisabled, ServerDisconnected, ServerConnecting, ServerConnected,
		ServerFailed, ServerNeedsAuth:
		return true
	default:
		return false
	}
}

// String returns the stable server-state name.
func (s ServerStateType) String() string {
	if !s.Valid() {
		return "unknown"
	}
	return string(s)
}

// ServerStatus is the application status notification read model. Known is
// false after a removed server's final invalidation.
type ServerStatus struct {
	Name      mcpserver.ServerName
	Known     bool
	State     mcpserver.ConnectionState
	ToolCount *int
}

func (s ServerStatus) Validate() error {
	if err := s.Name.Validate(); err != nil {
		return fmt.Errorf("mcp: invalid server status: %w", err)
	}
	if !s.Known {
		if s.State != "" || s.ToolCount != nil {
			return errors.New("mcp: unknown server status carries live state")
		}
		return nil
	}
	if err := s.State.Validate(); err != nil {
		return err
	}
	if s.State == mcpserver.ConnectionConnected {
		if s.ToolCount == nil {
			return errors.New("mcp: connected server status has no tool count")
		}
		if err := mcpserver.ValidateRemoteToolCount(*s.ToolCount); err != nil {
			return err
		}
		return nil
	}
	if s.ToolCount != nil {
		return fmt.Errorf("mcp: server status %q carries a tool count", s.State)
	}
	return nil
}

// TestResult is the semantic outcome of a non-persisting connection probe.
type TestResult struct {
	OK bool
}

func connectionView(server mcpserver.Server) Connection {
	return Connection{
		Transport:           server.Transport,
		URL:                 server.URL,
		AuthorizationMasked: secrets.Mask(server.Authorization),
		HeadersMasked:       maskedValues(server.Headers),
		Command:             server.Command,
		Args:                slices.Clone(server.Args),
		EnvironmentMasked:   maskedValues(server.Env),
		Dir:                 server.Dir,
	}
}

func maskedValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	masked := make(map[string]string, len(values))
	for key, value := range values {
		masked[key] = secrets.Mask(value)
	}
	return masked
}

func serverView(server mcpserver.Server, status *ServerStatus) Server {
	view := Server{
		Name:             server.Name,
		Description:      server.Description,
		Connection:       connectionView(server),
		HandshakeTimeout: server.HandshakeTimeout,
		ToolPolicy:       server.ToolPolicy,
		State:            ServerState{Type: ServerDisconnected},
	}
	if !server.Enabled {
		view.State.Type = ServerDisabled
		return view
	}
	if status == nil || !status.Known {
		return view
	}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State.Type = ServerConnecting
	case mcpserver.ConnectionConnected:
		view.State.Type = ServerConnected
		view.State.ToolCount = status.ToolCount
	case mcpserver.ConnectionFailed:
		view.State.Type = ServerFailed
	case mcpserver.ConnectionNeedsAuth:
		view.State.Type = ServerNeedsAuth
	default:
		panic("mcp: unknown MCP connection state")
	}
	return view
}

func statusView(status mcpserver.ConnectionStatus) (ServerStatus, error) {
	if err := status.Validate(); err != nil {
		return ServerStatus{}, err
	}
	view := ServerStatus{Name: status.Name, Known: true, State: status.State}
	if status.State == mcpserver.ConnectionConnected {
		count := status.ToolCount
		view.ToolCount = &count
	}
	return view, nil
}
