package mcpserver

import (
	"errors"
	"maps"
)

var ErrOAuthSessionSuperseded = errors.New("mcp: oauth session superseded")

// OAuthTarget identifies the configuration that requested a credential grant.
// The persisted server must still authorize it when the grant is issued or read.
type OAuthTarget struct {
	Server  ServerName
	URL     string
	Headers map[string]string
}

func (o OAuthTarget) Matches(server Server) bool {
	return server.Enabled &&
		server.Transport == TransportStreamableHTTP && server.Authorization == "" &&
		o.Equal(OAuthTarget{Server: server.Name, URL: server.URL, Headers: server.Headers})
}

func (o OAuthTarget) Equal(other OAuthTarget) bool {
	return o.Server == other.Server && o.URL == other.URL && maps.Equal(o.Headers, other.Headers)
}
