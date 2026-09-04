// Package mcpserver models user-defined MCP server connections. It owns server
// identity, transport configuration, enablement, credential-bearing fields,
// and per-tool policy; connection lifecycle and persistence are outside this
// package.
package mcpserver

import (
	"fmt"
	"maps"
	"slices"
)

// Transport names an MCP server connection mode using the standard
// `mcpServers` vocabulary. It is shared by persisted and live domain values.
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamableHttp"
)

// Server is one registry entry: an MCP server descriptor plus its enablement
// and per-tool gating. Name is the primary key and the prefix that namespaces
// the server's tools ("<name>_<tool>") across servers.
type Server struct {
	// Name identifies the server and namespaces its tools. Required, unique.
	Name ServerName

	// Transport is [TransportStdio] or [TransportStreamableHTTP]. Required.
	Transport Transport

	// Enabled gates whether the server is dialed. A disabled server stays in
	// the registry but contributes no tools.
	Enabled bool

	// Description is an optional human note.
	Description string

	// URL is the Streamable HTTP endpoint. Used when Transport == [TransportStreamableHTTP].
	URL string

	// Authorization, when set, is sent as the HTTP `Authorization` header
	// (typically "Bearer <token>") — HTTP transport only. It is sensitive and
	// must never be logged or exposed without masking. [Server.Headers] cannot
	// carry a second Authorization representation.
	Authorization string

	// Headers carries extra static HTTP request headers (e.g. "X-API-Key") sent
	// on every request — HTTP transport only. Values are sensitive and must
	// never be logged or exposed without masking because arbitrary headers may carry
	// credentials.
	Headers map[string]string

	// Command is the executable to spawn. Used when Transport == [TransportStdio].
	Command string

	// Args are the command arguments (stdio).
	Args []string

	// Env REPLACES the subprocess environment (stdio) as a KEY→value map; it does
	// not extend the parent env. Values are sensitive and must never be logged or
	// exposed without masking.
	Env map[string]string

	// Dir sets the subprocess working directory; empty inherits the parent's (stdio).
	Dir string

	// HandshakeTimeout bounds connection establishment for both transports. The
	// value object distinguishes an unbounded handshake from a bounded duration.
	HandshakeTimeout HandshakeTimeout

	// ToolPolicy owns the exact remote identities hidden from the model or
	// allowed to skip HITL. A tool cannot carry contradictory decisions.
	ToolPolicy ServerToolPolicy
}

// Clone returns an owned server snapshot across persistence and live-connection
// boundaries. ToolPolicy is already immutable and owns its rule relation.
func (s Server) Clone() Server {
	s.Headers = maps.Clone(s.Headers)
	s.Args = slices.Clone(s.Args)
	s.Env = maps.Clone(s.Env)
	return s
}

// Format keeps credential-bearing fields behind a redaction boundary for every
// fmt verb. Connection adapters reveal the raw fields explicitly; diagnostics
// and test failures must not do so accidentally through %+v or %#v.
func (s Server) Format(state fmt.State, _ rune) {
	timeout := "unbounded"
	if duration, bounded := s.HandshakeTimeout.Duration(); bounded {
		timeout = duration.String()
	}
	_, _ = fmt.Fprintf(
		state,
		"Server{Name:%q, Transport:%q, Enabled:%t, Description:%q, URL:%s, Authorization:%s, Headers:%s, Command:%q, Args:%q, Env:%s, Dir:%q, HandshakeTimeout:%s, ToolPolicyRules:%d}",
		s.Name,
		s.Transport,
		s.Enabled,
		s.Description,
		secretPresence(s.URL != ""),
		secretPresence(s.Authorization != ""),
		secretPresence(len(s.Headers) > 0),
		s.Command,
		s.Args,
		secretPresence(len(s.Env) > 0),
		s.Dir,
		timeout,
		len(s.ToolPolicy.Rules()),
	)
}

func secretPresence(present bool) string {
	if present {
		return "[REDACTED]"
	}
	return "<absent>"
}

// Validate reports whether the server is well-formed for its transport: the
// chosen transport's required field is set and the other transport's fields
// are blank before connection-specific state is attached.
func (s Server) Validate() error {
	if err := s.Name.Validate(); err != nil {
		return err
	}
	if err := s.HandshakeTimeout.Validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.Name, err)
	}
	if err := s.ToolPolicy.Validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.Name, err)
	}
	switch s.Transport {
	case TransportStreamableHTTP:
		if s.URL == "" {
			return fmt.Errorf("mcpserver %q: URL is required for streamableHttp transport", s.Name)
		}
		if s.Command != "" {
			return fmt.Errorf("mcpserver %q: Command must be empty for streamableHttp transport", s.Name)
		}
		if len(s.Args) > 0 {
			return fmt.Errorf("mcpserver %q: Args apply to stdio transport only", s.Name)
		}
		if len(s.Env) > 0 {
			return fmt.Errorf("mcpserver %q: Env applies to stdio transport only", s.Name)
		}
		if s.Dir != "" {
			return fmt.Errorf("mcpserver %q: Dir applies to stdio transport only", s.Name)
		}
		if err := validateHTTPConfiguration(s.Authorization, s.Headers); err != nil {
			return fmt.Errorf("mcpserver %q: %w", s.Name, err)
		}
	case TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("mcpserver %q: Command is required for stdio transport", s.Name)
		}
		if s.URL != "" {
			return fmt.Errorf("mcpserver %q: URL must be empty for stdio transport", s.Name)
		}
		if s.Authorization != "" {
			return fmt.Errorf("mcpserver %q: Authorization applies to http transport only", s.Name)
		}
		if len(s.Headers) > 0 {
			return fmt.Errorf("mcpserver %q: Headers apply to http transport only", s.Name)
		}
		if err := validateProcessConfiguration(s.Command, s.Args, s.Env, s.Dir); err != nil {
			return fmt.Errorf("mcpserver %q: %w", s.Name, err)
		}
	default:
		return fmt.Errorf("mcpserver %q: unknown transport %q (want %q or %q)", s.Name, s.Transport, TransportStdio, TransportStreamableHTTP)
	}
	return nil
}
