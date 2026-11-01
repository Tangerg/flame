// Package mcpserver models user-defined MCP server connections. It owns server
// identity, transport configuration, enablement, credential-bearing fields,
// and source authority; connection lifecycle and persistence are outside this
// package.
package mcpserver

import (
	"fmt"
)

// Transport names an MCP server connection mode using the standard
// `mcpServers` vocabulary. It is shared by persisted and live domain values.
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamableHttp"
)

// Server is one registry entry: an MCP server descriptor plus its enablement.
// Its [ID] is the registry key; the local Name also namespaces the server's
// tools ("<name>_<tool>") in the model-facing projection.
type Server struct {
	// Source is the record's owner and, for an installation, the release it
	// realizes. Required.
	Source Source
	// Name is unique within Source's origin. Required.
	Name ServerName

	// The connector supplies this revision to fence edits prepared from an earlier directory entry.
	DirectoryRevision string

	// Enabled gates whether the server is dialed. A disabled server stays in
	// the registry but contributes no tools.
	Enabled bool

	// Description is an optional human note.
	Description string

	// Connection is how the server is reached. Required.
	Connection Connection

	// HandshakeTimeout bounds connection establishment for both transports. The
	// value object distinguishes an unbounded handshake from a bounded duration.
	HandshakeTimeout HandshakeTimeout
}

func (s Server) ID() ID { return s.Source.ID(s.Name) }

// Clone returns an owned server snapshot across persistence and live-connection
// boundaries.
func (s Server) Clone() Server {
	if s.Connection != nil {
		s.Connection = s.Connection.clone()
	}
	return s
}

// Transport is the variant of the server's connection.
func (s Server) Transport() Transport {
	if s.Connection == nil {
		return ""
	}
	return s.Connection.Transport()
}

// Format keeps credential-bearing fields behind a redaction boundary for every
// fmt verb. Connection adapters reveal the raw fields explicitly; diagnostics
// and test failures must not do so accidentally through %+v or %#v.
func (s Server) Format(state fmt.State, _ rune) {
	timeout := "unbounded"
	if duration, bounded := s.HandshakeTimeout.Duration(); bounded {
		timeout = duration.String()
	}
	var connection string
	switch c := s.Connection.(type) {
	case StdioConnection:
		connection = fmt.Sprintf("Command:%q, Args:%q, Env:%s, Dir:%q", c.Command, c.Args, SecretPresence(len(c.Env) > 0), c.Dir)
	case HTTPConnection:
		connection = fmt.Sprintf("URL:%s, Authorization:%s, Headers:%s", SecretPresence(c.URL != ""), SecretPresence(c.Authorization != ""), SecretPresence(len(c.Headers) > 0))
	}
	_, _ = fmt.Fprintf(state, "Server{ID:%q, Transport:%q, Enabled:%t, Description:%q, %s, HandshakeTimeout:%s}",
		s.ID(), s.Transport(), s.Enabled, s.Description, connection, timeout)
}

// SecretPresence spells how a credential-bearing field appears in diagnostics.
// The connection adapter carries the same fields and reports them the same way,
// so a reader compares two renderings of one server rather than two vocabularies.
func SecretPresence(present bool) string {
	if present {
		return "[REDACTED]"
	}
	return "<absent>"
}

// Validate reports whether the server is well-formed for its transport.
func (s Server) Validate() error {
	if err := s.Source.Validate(); err != nil {
		return err
	}
	if err := s.Name.Validate(); err != nil {
		return err
	}
	return s.ValidateConnection()
}

// ValidateConnection checks the descriptor alone, for an owner that admits a
// declaration before the record it will become has an identity.
func (s Server) ValidateConnection() error {
	if err := s.HandshakeTimeout.Validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.ID(), err)
	}
	if s.Connection == nil {
		return fmt.Errorf("mcpserver %q: a connection is required", s.ID())
	}
	if err := s.Connection.validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.ID(), err)
	}
	return nil
}
