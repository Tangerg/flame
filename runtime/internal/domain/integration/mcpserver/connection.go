package mcpserver

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
)

// Connection is the closed set of ways to reach an MCP server. The variant is
// the transport, so a descriptor cannot carry another transport's fields.
type Connection interface {
	Transport() Transport
	validate() error
	clone() Connection
	// authorityFields frames everything except credentials: a token rotation
	// does not grant authority to a different endpoint.
	authorityFields() []string
	sameAs(Connection) bool
}

// StdioConnection spawns the server as a local subprocess.
type StdioConnection struct {
	Command string
	// ExecutionEnvironment captures a distribution-selected interpreter and
	// launch policy independently of plugin bytes and business command identity.
	ExecutionEnvironment string
	Args                 []string
	// Env replaces the subprocess environment. nil inherits the parent; an
	// empty map supplies no environment. Values are sensitive and must never
	// be logged or exposed without masking.
	Env map[string]string
	// Dir sets the working directory; empty inherits the parent's.
	Dir string
}

// HTTPConnection reaches the server over Streamable HTTP.
type HTTPConnection struct {
	URL string
	// Authorization, when set, is sent as the HTTP Authorization header. It is
	// sensitive and Headers cannot carry a second representation of it.
	Authorization string
	// Headers are extra static request headers. Values are sensitive because
	// arbitrary headers may carry credentials.
	Headers map[string]string
}

func (StdioConnection) Transport() Transport { return TransportStdio }
func (HTTPConnection) Transport() Transport  { return TransportStreamableHTTP }

func (c StdioConnection) validate() error {
	if c.Command == "" {
		return fmt.Errorf("Command is required for stdio transport")
	}
	return validateProcessConfiguration(c.Command, c.Args, c.Env, c.Dir)
}

func (c HTTPConnection) validate() error {
	if c.URL == "" {
		return fmt.Errorf("URL is required for streamableHttp transport")
	}
	if _, err := httporigin.ParseEndpoint(c.URL); err != nil {
		return fmt.Errorf("invalid HTTP endpoint: %w", err)
	}
	return ValidateHTTPHeaders(c.Authorization, c.Headers)
}

func (c StdioConnection) clone() Connection {
	c.Args = slices.Clone(c.Args)
	c.Env = maps.Clone(c.Env)
	return c
}

func (c HTTPConnection) clone() Connection {
	c.Headers = maps.Clone(c.Headers)
	return c
}

// Args are variable-length, so they come last behind their count.
func (c StdioConnection) authorityFields() []string {
	fields := []string{c.Command, c.Dir, c.ExecutionEnvironment, strconv.Itoa(len(c.Args))}
	return append(fields, c.Args...)
}

func (c HTTPConnection) authorityFields() []string { return []string{c.URL} }

func (c StdioConnection) sameAs(other Connection) bool {
	o, ok := other.(StdioConnection)
	return ok && c.Command == o.Command && c.ExecutionEnvironment == o.ExecutionEnvironment && c.Dir == o.Dir &&
		slices.Equal(c.Args, o.Args) && (c.Env == nil) == (o.Env == nil) && maps.Equal(c.Env, o.Env)
}

func (c HTTPConnection) sameAs(other Connection) bool {
	o, ok := other.(HTTPConnection)
	return ok && c.URL == o.URL && c.Authorization == o.Authorization && maps.Equal(c.Headers, o.Headers)
}
