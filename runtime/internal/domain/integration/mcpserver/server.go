package mcpserver

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/secret"
)

type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamableHttp"
)

type Server struct {
	Source Source
	Name   ServerName

	DirectoryRevision string

	Enabled bool

	Description string

	Connection Connection

	HandshakeTimeout HandshakeTimeout
}

func (s Server) ID() ID { return s.Source.ID(s.Name) }

func (s Server) Clone() Server {
	if s.Connection != nil {
		s.Connection = s.Connection.clone()
	}
	return s
}

func (s Server) Transport() Transport {
	if s.Connection == nil {
		return ""
	}
	return s.Connection.Transport()
}

// Credential-bearing fields stay redacted for every fmt verb so %+v or %#v in
// diagnostics cannot leak them.
func (s Server) Format(state fmt.State, _ rune) {
	timeout := "unbounded"
	if duration, bounded := s.HandshakeTimeout.Duration(); bounded {
		timeout = duration.String()
	}
	var connection string
	switch c := s.Connection.(type) {
	case StdioConnection:
		connection = fmt.Sprintf("Command:%q, Args:%q, Env:%s, Dir:%q", c.Command, c.Args, secret.Presence(len(c.Env) > 0), c.Dir)
	case HTTPConnection:
		connection = fmt.Sprintf("URL:%s, Authorization:%s, Headers:%s", secret.Presence(c.URL != ""), secret.Presence(c.Authorization != ""), secret.Presence(len(c.Headers) > 0))
	}
	_, _ = fmt.Fprintf(state, "Server{ID:%q, Transport:%q, Enabled:%t, Description:%q, %s, HandshakeTimeout:%s}",
		s.ID(), s.Transport(), s.Enabled, s.Description, connection, timeout)
}

func (s Server) Validate() error {
	if err := s.Source.Validate(); err != nil {
		return err
	}
	if err := s.Name.Validate(); err != nil {
		return err
	}
	return s.ValidateConnection()
}

func (s Server) ValidateConnection() error {
	if err := s.HandshakeTimeout.Validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.ID(), err)
	}
	switch s.Connection.(type) {
	case nil:
		return fmt.Errorf("mcpserver %q: a connection is required", s.ID())
	case StdioConnection, HTTPConnection:
	default:
		return fmt.Errorf("mcpserver %q: connection must be a value variant", s.ID())
	}
	if err := s.Connection.validate(); err != nil {
		return fmt.Errorf("mcpserver %q: %w", s.ID(), err)
	}
	return nil
}
