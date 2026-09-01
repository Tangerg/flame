// Package mcp defines the CLI-owned MCP server configuration, live status,
// tool catalog, and interactive authorization values.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/failure"
)

var (
	// ErrServerNotFound reports that the addressed configured server no longer exists.
	ErrServerNotFound = errors.New("MCP server not found")
	// ErrServerAlreadyExists reports a create conflict on the server identity.
	ErrServerAlreadyExists = errors.New("MCP server already exists")
	// ErrServerDisabled reports that a live operation requires an enabled server.
	ErrServerDisabled = errors.New("MCP server is disabled")
	// ErrAuthorizationAttemptNotFound reports an expired or unknown observation target.
	ErrAuthorizationAttemptNotFound = errors.New("MCP authorization attempt not found")
)

type State struct {
	Type      protocol.MCPServerStateType
	ToolCount *int
	Problem   *failure.Problem
}

func (s State) Validate() error {
	switch s.Type {
	case protocol.MCPServerDisabled, protocol.MCPServerDisconnected, protocol.MCPServerConnecting:
		if s.ToolCount != nil || s.Problem != nil {
			return fmt.Errorf("MCP %s state carries foreign data", s.Type)
		}
	case protocol.MCPServerConnected:
		if s.ToolCount == nil || *s.ToolCount < 0 || s.Problem != nil {
			return errors.New("connected MCP state requires a non-negative tool count and no problem")
		}
	case protocol.MCPServerFailed, protocol.MCPServerNeedsAuth:
		if s.ToolCount != nil || s.Problem == nil {
			return fmt.Errorf("MCP %s state requires only a problem", s.Type)
		}
		if err := s.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP state %q is invalid", s.Type)
	}
	return nil
}

type Connection struct {
	Transport           protocol.MCPTransport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Directory           string
}

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

func (h HandshakeTimeout) Equal(other HandshakeTimeout) bool {
	return h.bounded == other.bounded && h.seconds == other.seconds
}

func (h HandshakeTimeout) String() string {
	if seconds, bounded := h.Seconds(); bounded {
		return fmt.Sprintf("%ds", seconds)
	}
	return "unbounded"
}

func (c Connection) Validate() error {
	switch c.Transport {
	case protocol.MCPTransportStreamableHTTP:
		if strings.TrimSpace(c.URL) == "" {
			return errors.New("HTTP MCP connection URL is empty")
		}
		if c.Command != "" || len(c.Args) != 0 || len(c.EnvironmentMasked) != 0 || c.Directory != "" {
			return errors.New("HTTP MCP connection carries stdio fields")
		}
	case protocol.MCPTransportStdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio MCP connection command is empty")
		}
		if c.URL != "" || c.AuthorizationMasked != "" || len(c.HeadersMasked) != 0 {
			return errors.New("stdio MCP connection carries HTTP fields")
		}
	default:
		return fmt.Errorf("MCP transport %q is invalid", c.Transport)
	}
	if err := validateStringMap("masked MCP headers", c.HeadersMasked); err != nil {
		return err
	}
	return validateStringMap("masked MCP environment", c.EnvironmentMasked)
}

func (c Connection) Clone() Connection {
	c.Args = slices.Clone(c.Args)
	c.HeadersMasked = maps.Clone(c.HeadersMasked)
	c.EnvironmentMasked = maps.Clone(c.EnvironmentMasked)
	return c
}

type Server struct {
	Name             string
	Description      string
	Connection       Connection
	HandshakeTimeout HandshakeTimeout
	DisabledTools    []string
	AutoApproveTools []string
	State            State
}

func (s Server) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("MCP server name is empty")
	}
	if err := s.HandshakeTimeout.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", s.Name, err)
	}
	if err := s.Connection.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", s.Name, err)
	}
	if err := s.State.Validate(); err != nil {
		return fmt.Errorf("MCP server %s: %w", s.Name, err)
	}
	return validateCanonicalToolPolicy(s.DisabledTools, s.AutoApproveTools)
}

func (s Server) Clone() Server {
	s.Connection = s.Connection.Clone()
	s.DisabledTools = slices.Clone(s.DisabledTools)
	s.AutoApproveTools = slices.Clone(s.AutoApproveTools)
	if s.State.ToolCount != nil {
		s.State.ToolCount = new(*s.State.ToolCount)
	}
	if s.State.Problem != nil {
		s.State.Problem = s.State.Problem.Clone()
	}
	return s
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

func (c Candidate) ValidateResult(result Server) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != c.Name {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, c.Name))
	}
	if result.Description != c.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, c.Description))
	}
	if !result.HandshakeTimeout.Equal(c.HandshakeTimeout) {
		problems = append(problems, fmt.Errorf("runtime returned handshake timeout %s, want %s", result.HandshakeTimeout, c.HandshakeTimeout))
	}
	if !equalToolNameSet(result.DisabledTools, c.DisabledTools) {
		problems = append(problems, fmt.Errorf("runtime returned disabled tools %v, want %v", result.DisabledTools, c.DisabledTools))
	}
	if !equalToolNameSet(result.AutoApproveTools, c.AutoApproveTools) {
		problems = append(problems, fmt.Errorf("runtime returned auto-approved tools %v, want %v", result.AutoApproveTools, c.AutoApproveTools))
	}
	problems = append(problems, validateEnabledResult(c.Enabled, result.State))
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

func (s ServerUpdate) ValidateResult(result Server) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Name != s.Server {
		problems = append(problems, fmt.Errorf("runtime returned server %q, want %q", result.Name, s.Server))
	}
	if s.Enabled != nil {
		problems = append(problems, validateEnabledResult(*s.Enabled, result.State))
	}
	if s.Description != nil && result.Description != *s.Description {
		problems = append(problems, fmt.Errorf("runtime returned description %q, want %q", result.Description, *s.Description))
	}
	if s.HandshakeTimeout != nil && !result.HandshakeTimeout.Equal(*s.HandshakeTimeout) {
		problems = append(problems, fmt.Errorf("runtime returned handshake timeout %s, want %s", result.HandshakeTimeout, *s.HandshakeTimeout))
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

func validateEnabledResult(enabled bool, state State) error {
	disabled := state.Type == protocol.MCPServerDisabled
	if disabled == enabled {
		return fmt.Errorf("runtime returned state %q for enabled=%t", state.Type, enabled)
	}
	return nil
}

func (c ConnectionInput) validateCreateResult(result Connection) error {
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
		return validateMaskedMap("environment", c.Environment, result.EnvironmentMasked)
	default:
		return nil
	}
}

func (c ConnectionInput) validateUpdateResult(result Connection) error {
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
			return validateMaskedMap("environment", c.Environment, result.EnvironmentMasked)
		}
	}
	return nil
}

func (c ConnectionInput) validateVisibleResult(result Connection) error {
	var problems []error
	if result.Transport != c.Transport {
		problems = append(problems, fmt.Errorf("runtime returned transport %q, want %q", result.Transport, c.Transport))
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
		if result.Directory != c.Directory {
			problems = append(problems, fmt.Errorf("runtime returned directory %q, want %q", result.Directory, c.Directory))
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

type TestResult struct {
	OK      bool
	Problem *failure.Problem
}

func (t TestResult) Validate() error {
	if t.OK == (t.Problem != nil) {
		return errors.New("MCP test result must contain exactly one success or problem state")
	}
	if t.Problem != nil {
		return t.Problem.Validate()
	}
	return nil
}

type Tool struct {
	Server      string
	Name        string
	Description string
	InputSchema json.RawMessage
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Server) == "" || strings.TrimSpace(t.Name) == "" {
		return errors.New("MCP tool requires server and name")
	}
	if len(t.InputSchema) != 0 && !json.Valid(t.InputSchema) {
		return fmt.Errorf("MCP tool %s/%s has invalid input schema JSON", t.Server, t.Name)
	}
	return nil
}

type AuthorizationAttempt struct {
	ID         string
	Server     string
	Status     protocol.MCPAuthorizationAttemptStatusType
	Problem    *failure.Problem
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// AuthorizationReference is the stable identity used to observe an attempt.
// Server is retained even though the runtime query is keyed by ID so adapters
// can reject a response that silently crosses authorization ownership.
type AuthorizationReference struct {
	ID     string
	Server string
}

func (a AuthorizationReference) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Server) == "" {
		return errors.New("MCP authorization reference requires attempt id and server")
	}
	return nil
}

func (a AuthorizationAttempt) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Server) == "" || a.CreatedAt.IsZero() {
		return errors.New("MCP authorization attempt identity is incomplete")
	}
	switch a.Status {
	case protocol.MCPAuthorizationAttemptPending:
		if a.Problem != nil || a.FinishedAt != nil {
			return errors.New("pending MCP authorization carries a terminal result")
		}
	case protocol.MCPAuthorizationAttemptSucceeded, protocol.MCPAuthorizationAttemptCanceled:
		if a.Problem != nil || a.FinishedAt == nil {
			return fmt.Errorf("%s MCP authorization has an invalid terminal result", a.Status)
		}
	case protocol.MCPAuthorizationAttemptFailed:
		if a.Problem == nil || a.FinishedAt == nil {
			return errors.New("failed MCP authorization requires a problem and finish time")
		}
		if err := a.Problem.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("MCP authorization status %q is invalid", a.Status)
	}
	if a.FinishedAt != nil && a.FinishedAt.Before(a.CreatedAt) {
		return errors.New("MCP authorization finished before it started")
	}
	return nil
}

func (a AuthorizationAttempt) Pending() bool {
	return a.Status == protocol.MCPAuthorizationAttemptPending
}

func (a AuthorizationAttempt) Reference() AuthorizationReference {
	return AuthorizationReference{ID: a.ID, Server: a.Server}
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
