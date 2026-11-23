package mcpserver

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
)

type Connection interface {
	Transport() Transport
	validate() error
	clone() Connection
	authorityFields() []string
	sameAs(Connection) bool
}

type StdioConnection struct {
	Command              string
	ExecutionEnvironment string
	Args                 []string
	// nil inherits the parent environment; an empty map supplies none. Values are
	// sensitive.
	Env map[string]string
	Dir string
}

type HTTPConnection struct {
	URL           string
	Authorization string
	Headers       map[string]string
}

func (StdioConnection) Transport() Transport { return TransportStdio }
func (HTTPConnection) Transport() Transport  { return TransportStreamableHTTP }

func (c StdioConnection) validate() error {
	if c.Command == "" {
		return errors.New("command is required for stdio transport")
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
