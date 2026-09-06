// Package mcp defines CLI-owned MCP authoring and acknowledgement checks.
// Runtime protocol values carry server, tool, and authorization observations.
package mcp

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

// HandshakeTimeout is the CLI's MCP connection deadline policy. Its zero value
// is explicitly unbounded; a bounded value can only be constructed with a
// strictly positive number of seconds.
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

func (h HandshakeTimeout) IsBounded() bool { return h.bounded }

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

// Matches compares authored intent with the Runtime acknowledgement.
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

// ValidateServer checks the Runtime shape and the catalog relationships that
// span independent wire fields.
func ValidateServer(server protocol.MCPServer) error {
	if err := protocol.ValidateWireTree(server); err != nil {
		return err
	}
	connection := server.Connection
	if connection.Type == protocol.MCPTransportStreamableHTTP && strings.TrimSpace(connection.URL) == "" {
		return errors.New("HTTP MCP connection URL is empty")
	}
	if connection.Type == protocol.MCPTransportStdio && strings.TrimSpace(connection.Command) == "" {
		return errors.New("stdio MCP connection command is empty")
	}
	if err := validateStringMap("masked MCP headers", connection.HeadersMasked); err != nil {
		return err
	}
	if err := validateStringMap("masked MCP environment", connection.EnvMasked); err != nil {
		return err
	}
	return validateCanonicalToolPolicy(server.DisabledTools, server.AutoApproveTools)
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
	return validateMapChange("MCP headers", h.Kind, h.Value)
}

type EnvironmentChange struct {
	Kind  protocol.MCPSecretChangeType
	Value map[string]string
}

func (e EnvironmentChange) Validate() error {
	return validateMapChange("MCP environment", e.Kind, e.Value)
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
			if err := c.Headers.Validate(); err != nil {
				return err
			}
		}
	case protocol.MCPTransportStdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio MCP connection input command is empty")
		}
		if c.URL != "" || c.Authorization != nil || c.Headers != nil {
			return errors.New("stdio MCP connection input carries HTTP fields")
		}
		if c.Environment != nil {
			if err := c.Environment.Validate(); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("MCP transport %q is invalid", c.Transport)
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
	DisabledTools    []string
	AutoApproveTools []string
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
	return validateToolPolicy(c.DisabledTools, c.AutoApproveTools)
}

func (c Candidate) Clone() Candidate {
	c.Connection = c.Connection.Clone()
	c.DisabledTools = slices.Clone(c.DisabledTools)
	c.AutoApproveTools = slices.Clone(c.AutoApproveTools)
	return c
}

func (c Candidate) ValidateResult(result protocol.MCPServer) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := ValidateServer(result); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != c.Name {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, c.Name))
	}
	if result.Description != c.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, c.Description))
	}
	if !c.HandshakeTimeout.Matches(result.HandshakeTimeout) {
		problems = append(problems, fmt.Errorf("runtime did not confirm handshake timeout %s", c.HandshakeTimeout))
	}
	if !equalToolNameSet(result.DisabledTools, c.DisabledTools) {
		problems = append(problems, fmt.Errorf("runtime returned disabled tools %v, want %v", result.DisabledTools, c.DisabledTools))
	}
	if !equalToolNameSet(result.AutoApproveTools, c.AutoApproveTools) {
		problems = append(problems, fmt.Errorf("runtime returned auto-approved tools %v, want %v", result.AutoApproveTools, c.AutoApproveTools))
	}
	problems = append(problems, validateEnabledResult(c.Enabled, result.Status))
	problems = append(problems, c.Connection.validateCreateResult(result.Connection))
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP candidate %s: %w", c.Name, err)
	}
	return nil
}

type ServerUpdate struct {
	Server           string
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	HandshakeTimeout *HandshakeTimeout
	DisabledTools    *[]string
	AutoApproveTools *[]string
}

func (s ServerUpdate) Validate() error {
	if strings.TrimSpace(s.Server) == "" {
		return errors.New("MCP update server is empty")
	}
	if !s.HasChanges() {
		return errors.New("MCP update has no changes")
	}
	if s.Connection != nil {
		if err := s.Connection.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", s.Server, err)
		}
	}
	if s.HandshakeTimeout != nil {
		if err := s.HandshakeTimeout.Validate(); err != nil {
			return fmt.Errorf("MCP update %s: %w", s.Server, err)
		}
	}
	if s.DisabledTools != nil {
		if err := validateUniqueStrings("disabled MCP tools", *s.DisabledTools); err != nil {
			return err
		}
	}
	if s.AutoApproveTools != nil {
		if err := validateUniqueStrings("auto-approved MCP tools", *s.AutoApproveTools); err != nil {
			return err
		}
	}
	if s.DisabledTools != nil && s.AutoApproveTools != nil {
		if err := validateToolPolicy(*s.DisabledTools, *s.AutoApproveTools); err != nil {
			return err
		}
	}
	return nil
}

func (s ServerUpdate) HasChanges() bool {
	return s.Enabled != nil || s.Description != nil || s.Connection != nil || s.HandshakeTimeout != nil ||
		s.DisabledTools != nil || s.AutoApproveTools != nil
}

func (s ServerUpdate) ValidateResult(result protocol.MCPServer) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := ValidateServer(result); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != s.Server {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, s.Server))
	}
	if s.Enabled != nil {
		problems = append(problems, validateEnabledResult(*s.Enabled, result.Status))
	}
	if s.Description != nil && result.Description != *s.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, *s.Description))
	}
	if s.HandshakeTimeout != nil && !s.HandshakeTimeout.Matches(result.HandshakeTimeout) {
		problems = append(problems, fmt.Errorf("runtime did not confirm handshake timeout %s", *s.HandshakeTimeout))
	}
	if s.DisabledTools != nil && !equalToolNameSet(result.DisabledTools, *s.DisabledTools) {
		problems = append(problems, fmt.Errorf("runtime returned disabled tools %v, want %v", result.DisabledTools, *s.DisabledTools))
	}
	if s.AutoApproveTools != nil && !equalToolNameSet(result.AutoApproveTools, *s.AutoApproveTools) {
		problems = append(problems, fmt.Errorf("runtime returned auto-approved tools %v, want %v", result.AutoApproveTools, *s.AutoApproveTools))
	}
	if s.Connection != nil {
		problems = append(problems, s.Connection.validateUpdateResult(result.Connection))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP update %s: %w", s.Server, err)
	}
	return nil
}

func validateEnabledResult(enabled bool, state protocol.MCPServerState) error {
	disabled := state.Type == protocol.MCPServerDisabled
	if disabled == enabled {
		return fmt.Errorf("runtime returned state %q for enabled=%t", state.Type, enabled)
	}
	return nil
}

func (c ConnectionInput) validateCreateResult(result protocol.MCPConnection) error {
	if err := c.validateVisibleResult(result); err != nil {
		return err
	}
	switch c.Transport {
	case protocol.MCPTransportStreamableHTTP:
		if err := validateMaskedSecret("authorization", c.Authorization, result.AuthorizationMasked); err != nil {
			return err
		}
		return validateMaskedMap("headers", c.Headers, result.HeadersMasked)
	case protocol.MCPTransportStdio:
		return validateMaskedMap("environment", c.Environment, result.EnvMasked)
	default:
		return nil
	}
}

func (c ConnectionInput) validateUpdateResult(result protocol.MCPConnection) error {
	if err := c.validateVisibleResult(result); err != nil {
		return err
	}
	switch c.Transport {
	case protocol.MCPTransportStreamableHTTP:
		if c.Authorization != nil {
			if err := validateMaskedSecret("authorization", c.Authorization, result.AuthorizationMasked); err != nil {
				return err
			}
		}
		if c.Headers != nil {
			return validateMaskedMap("headers", c.Headers, result.HeadersMasked)
		}
	case protocol.MCPTransportStdio:
		if c.Environment != nil {
			return validateMaskedMap("environment", c.Environment, result.EnvMasked)
		}
	}
	return nil
}

func (c ConnectionInput) validateVisibleResult(result protocol.MCPConnection) error {
	var problems []error
	if result.Type != c.Transport {
		problems = append(problems, fmt.Errorf("runtime returned transport %q, want %q", result.Type, c.Transport))
	}
	switch c.Transport {
	case protocol.MCPTransportStreamableHTTP:
		if result.URL != c.URL {
			problems = append(problems, fmt.Errorf("runtime returned URL %q, want %q", result.URL, c.URL))
		}
	case protocol.MCPTransportStdio:
		if result.Command != c.Command {
			problems = append(problems, fmt.Errorf("runtime returned command %q, want %q", result.Command, c.Command))
		}
		if !slices.Equal(result.Args, c.Args) {
			problems = append(problems, fmt.Errorf("runtime returned args %v, want %v", result.Args, c.Args))
		}
		if result.Dir != c.Directory {
			problems = append(problems, fmt.Errorf("runtime returned directory %q, want %q", result.Dir, c.Directory))
		}
	}
	return errors.Join(problems...)
}

func validateMaskedSecret(label string, change *AuthorizationChange, masked string) error {
	switch {
	case change == nil && masked != "":
		return fmt.Errorf("runtime returned unexpected masked %s", label)
	case change != nil && change.Kind == protocol.MCPSecretSet && masked == "":
		return fmt.Errorf("runtime did not confirm masked %s", label)
	case change != nil && change.Kind == protocol.MCPSecretClear && masked != "":
		return fmt.Errorf("runtime kept masked %s after clear", label)
	default:
		return nil
	}
}

func validateMaskedMap[T interface {
	HeadersChange | EnvironmentChange
}](label string, raw *T, masked map[string]string) error {
	if raw == nil {
		if len(masked) != 0 {
			return fmt.Errorf("runtime returned unexpected masked %s", label)
		}
		return nil
	}
	var kind protocol.MCPSecretChangeType
	var values map[string]string
	switch change := any(*raw).(type) {
	case HeadersChange:
		kind, values = change.Kind, change.Value
	case EnvironmentChange:
		kind, values = change.Kind, change.Value
	}
	if kind == protocol.MCPSecretClear {
		if len(masked) != 0 {
			return fmt.Errorf("runtime kept masked %s after clear", label)
		}
		return nil
	}
	if len(masked) != len(values) {
		return fmt.Errorf(
			"runtime returned masked %s keys %v, want %v",
			label,
			slices.Sorted(maps.Keys(masked)),
			slices.Sorted(maps.Keys(values)),
		)
	}
	for key := range values {
		if masked[key] == "" {
			return fmt.Errorf("runtime did not confirm masked %s key %q", label, key)
		}
	}
	return nil
}

// AuthorizationReference is the stable identity used to observe an attempt.
// Server is retained even though the runtime query is keyed by ID so adapters
// can reject a response that silently crosses authorization ownership.
type AuthorizationReference struct {
	ID     string
	Server string
}

func (a AuthorizationReference) Validate() error {
	var problems []error
	if err := (protocol.MCPAuthorizationAttemptRequest{AttemptID: a.ID}).ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := (protocol.MCPServerRequest{Server: a.Server}).ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("MCP authorization reference: %w", err)
	}
	return nil
}

// ValidateAuthorizationAttempt composes the Runtime wire contract with the
// observer's cross-timestamp chronological requirement.
func ValidateAuthorizationAttempt(attempt protocol.MCPAuthorizationAttempt) error {
	if err := protocol.ValidateWireTree(attempt); err != nil {
		return fmt.Errorf("MCP authorization attempt: %w", err)
	}
	if attempt.FinishedAt != nil && attempt.FinishedAt.Before(attempt.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

// AuthorizationReferenceFrom retains both identities needed to detect a poll
// response that crosses server ownership.
func AuthorizationReferenceFrom(attempt protocol.MCPAuthorizationAttempt) AuthorizationReference {
	return AuthorizationReference{ID: attempt.ID, Server: attempt.Server}
}

func validateMapChange(label string, kind protocol.MCPSecretChangeType, values map[string]string) error {
	switch kind {
	case protocol.MCPSecretClear:
		if len(values) != 0 {
			return fmt.Errorf("%s clear carries values", label)
		}
	case protocol.MCPSecretSet:
		if len(values) == 0 {
			return fmt.Errorf("%s set value is empty", label)
		}
	default:
		return fmt.Errorf("MCP secret change %q is invalid", kind)
	}
	return validateStringMap(label, values)
}

func validateStringMap(label string, values map[string]string) error {
	for key, value := range values {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty name or value", label)
		}
	}
	return nil
}

func validateUniqueStrings(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeats %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// validateToolPolicy checks the two-list command projection as one relation:
// one remote tool may have at most one policy decision. Input order is not
// semantic; Runtime owns the canonical sorted projection returned by reads.
func validateToolPolicy(disabled, autoApproved []string) error {
	if err := validateUniqueStrings("disabled MCP tools", disabled); err != nil {
		return err
	}
	if err := validateUniqueStrings("auto-approved MCP tools", autoApproved); err != nil {
		return err
	}
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, tool := range disabled {
		disabledSet[tool] = struct{}{}
	}
	for _, tool := range autoApproved {
		if _, contradictory := disabledSet[tool]; contradictory {
			return fmt.Errorf("MCP tool %q is both disabled and auto-approved", tool)
		}
	}
	return nil
}

func validateCanonicalToolPolicy(disabled, autoApproved []string) error {
	if err := validateToolPolicy(disabled, autoApproved); err != nil {
		return err
	}
	if !slices.IsSorted(disabled) || !slices.IsSorted(autoApproved) {
		return errors.New("MCP tool policy is not in canonical order")
	}
	return nil
}

func equalToolNameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
