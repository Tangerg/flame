package mcp

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type HandshakeTimeout struct {
	bounded bool
	seconds int
}

func NewHandshakeTimeout(seconds int) (HandshakeTimeout, error) {
	if seconds <= 0 {
		return HandshakeTimeout{}, errors.New("MCP handshake timeout must be a positive integer")
	}
	return HandshakeTimeout{bounded: true, seconds: seconds}, nil
}

func (h HandshakeTimeout) Seconds() (int, bool) {
	if !h.bounded {
		return 0, false
	}
	return h.seconds, true
}

func (h HandshakeTimeout) Validate() error {
	if !h.bounded {
		if h.seconds != 0 {
			return errors.New("unbounded MCP handshake timeout carries seconds")
		}
		return nil
	}
	if h.seconds <= 0 {
		return errors.New("bounded MCP handshake timeout must be positive")
	}
	return nil
}

func (h HandshakeTimeout) Matches(other protocol.MCPHandshakeTimeout) bool {
	if !h.bounded {
		return other.Type == protocol.MCPHandshakeUnbounded && other.Seconds == nil
	}
	return other.Type == protocol.MCPHandshakeBounded && other.Seconds != nil && h.seconds == *other.Seconds
}

func (h HandshakeTimeout) String() string {
	if seconds, bounded := h.Seconds(); bounded {
		return fmt.Sprintf("%ds", seconds)
	}
	return "unbounded"
}

type AuthorizationChange struct {
	Kind  protocol.MCPSecretChangeType
	Value string
}

func (a AuthorizationChange) Validate() error {
	switch a.Kind {
	case protocol.MCPSecretSet:
		if strings.TrimSpace(a.Value) == "" {
			return errors.New("MCP authorization set value is empty")
		}
	case protocol.MCPSecretClear:
		if a.Value != "" {
			return errors.New("MCP authorization clear carries a value")
		}
	default:
		return fmt.Errorf("MCP secret change %q is invalid", a.Kind)
	}
	return nil
}

type HeadersChange struct {
	Kind  protocol.MCPSecretChangeType
	Value map[string]string
}

func (h HeadersChange) Validate() error {
	return protocol.ValidateWireTree(protocol.MCPHeadersChange{Type: h.Kind, Value: h.Value})
}

type EnvironmentChange struct {
	Kind  protocol.MCPSecretChangeType
	Value map[string]string
}

func (e EnvironmentChange) Validate() error {
	return protocol.ValidateWireTree(protocol.MCPEnvironmentChange{Type: e.Kind, Value: e.Value})
}

type ConnectionInput struct {
	Transport     protocol.MCPTransport
	URL           string
	Authorization *AuthorizationChange
	Headers       *HeadersChange
	Command       string
	Args          []string
	Environment   *EnvironmentChange
	Directory     string
}

func (c ConnectionInput) Validate() error {
	switch c.Transport {
	case protocol.MCPTransportStreamableHTTP:
		return c.validateHTTP()
	case protocol.MCPTransportStdio:
		return c.validateStdio()
	default:
		return fmt.Errorf("MCP transport %q is invalid", c.Transport)
	}
}

func (c ConnectionInput) validateHTTP() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("HTTP MCP connection input URL is empty")
	}
	if c.Command != "" || len(c.Args) != 0 || c.Environment != nil || c.Directory != "" {
		return errors.New("HTTP MCP connection input carries stdio fields")
	}
	if c.Authorization != nil {
		if err := c.Authorization.Validate(); err != nil {
			return err
		}
	}
	if c.Headers != nil {
		return c.Headers.Validate()
	}
	return nil
}

func (c ConnectionInput) validateStdio() error {
	if c.Command == "" {
		return errors.New("stdio MCP connection input command is empty")
	}
	if c.URL != "" || c.Authorization != nil || c.Headers != nil {
		return errors.New("stdio MCP connection input carries HTTP fields")
	}
	if c.Environment != nil {
		return c.Environment.Validate()
	}
	return nil
}

func (c ConnectionInput) Clone() ConnectionInput {
	c.Args = slices.Clone(c.Args)
	if c.Authorization != nil {
		c.Authorization = new(*c.Authorization)
	}
	if c.Headers != nil {
		cloned := *c.Headers
		cloned.Value = maps.Clone(c.Headers.Value)
		c.Headers = &cloned
	}
	if c.Environment != nil {
		cloned := *c.Environment
		cloned.Value = maps.Clone(c.Environment.Value)
		c.Environment = &cloned
	}
	return c
}

func (c ConnectionInput) validateCandidateSecrets() error {
	if c.Authorization != nil && c.Authorization.Kind == protocol.MCPSecretClear {
		return errors.New("MCP candidate cannot clear authorization without an existing server")
	}
	if c.Headers != nil && c.Headers.Kind == protocol.MCPSecretClear {
		return errors.New("MCP candidate cannot clear headers without an existing server")
	}
	if c.Environment != nil && c.Environment.Kind == protocol.MCPSecretClear {
		return errors.New("MCP candidate cannot clear environment without an existing server")
	}
	return nil
}

type Candidate struct {
	Name             string
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	HandshakeTimeout HandshakeTimeout
}

func (c Candidate) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("MCP candidate name is empty")
	}
	if err := c.HandshakeTimeout.Validate(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	if err := c.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	if err := c.Connection.validateCandidateSecrets(); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	return nil
}

func (c Candidate) Clone() Candidate {
	c.Connection = c.Connection.Clone()
	return c
}

func UserServer(name string) protocol.MCPServerID {
	return protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: name}
}

func ParseServerReference(text string) (protocol.MCPServerID, error) {
	text = strings.TrimSpace(text)
	id := UserServer(text)
	if installation, name, found := strings.Cut(text, "/"); found {
		id = protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installation}, Name: name}
	}
	if err := protocol.ValidateWireTree(id); err != nil {
		return protocol.MCPServerID{}, fmt.Errorf("MCP server %q: %w", text, err)
	}
	return id, nil
}

func ServerLabel(id protocol.MCPServerID) string {
	if id.Origin.Type == protocol.MCPOriginInstallation {
		return id.Origin.InstallationID + "/" + id.Name
	}
	return id.Name
}

func ToolLabel(ref protocol.ToolRef) (string, error) {
	switch ref.Type {
	case protocol.ToolRefBuiltIn:
		return "builtIn/" + ref.Name, nil
	case protocol.ToolRefMCP:
		if ref.Server == nil {
			return "", fmt.Errorf("runtime contract violation: MCP tool %q has no server", ref.Name)
		}
		return "mcp/" + ServerLabel(*ref.Server) + "/" + ref.Name, nil
	default:
		return "", fmt.Errorf("runtime contract violation: unknown tool source %q", ref.Type)
	}
}

type ServerUpdate struct {
	Server           protocol.MCPServerID
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	HandshakeTimeout *HandshakeTimeout
}

func (s ServerUpdate) Validate() error {
	if err := protocol.ValidateWireTree(s.Server); err != nil {
		return fmt.Errorf("MCP update server: %w", err)
	}
	if !s.HasChanges() {
		return errors.New("MCP update has no changes")
	}
	if s.Connection != nil {
		if err := s.Connection.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", ServerLabel(s.Server), err)
		}
	}
	if s.HandshakeTimeout != nil {
		if err := s.HandshakeTimeout.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", ServerLabel(s.Server), err)
		}
	}

	return nil
}

func (s ServerUpdate) HasChanges() bool {
	return s.Enabled != nil || s.Description != nil || s.Connection != nil || s.HandshakeTimeout != nil
}

// Server is retained although the query is keyed by ID so adapters can reject a
// response that crosses authorization ownership.
type AuthorizationReference struct {
	ID     string
	Server protocol.MCPServerID
}

func (a AuthorizationReference) Validate() error {
	var problems []error
	if err := protocol.ValidateWireTree((protocol.MCPAuthorizationAttemptRequest{AttemptID: a.ID})); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateWireTree(protocol.MCPServerRequest{Server: a.Server}); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP authorization reference: %w", err)
	}
	return nil
}

func ValidateAuthorizationAttempt(attempt protocol.MCPAuthorizationAttempt) error {
	if attempt.FinishedAt != nil && attempt.FinishedAt.Before(attempt.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

func AuthorizationReferenceFrom(attempt protocol.MCPAuthorizationAttempt) AuthorizationReference {
	return AuthorizationReference{ID: attempt.ID, Server: attempt.Server}
}
