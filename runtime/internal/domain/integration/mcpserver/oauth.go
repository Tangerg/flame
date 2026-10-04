package mcpserver

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

var ErrOAuthSessionSuperseded = errors.New("mcp: oauth session superseded")

// OAuthTarget identifies the configuration that requested a credential grant.
// The persisted server must still authorize it when the grant is issued or read.
type OAuthTarget struct {
	Authority string
	Server    ServerName
	URL       string
	Headers   map[string]string
}

// Fingerprint binds a persisted grant to the complete requesting configuration,
// including headers. An origin alone cannot distinguish two credential targets.
func (o OAuthTarget) Fingerprint() string {
	fields := []string{o.Authority, o.Server.String(), o.URL}
	for _, name := range slices.Sorted(maps.Keys(o.Headers)) {
		fields = append(fields, name, o.Headers[name])
	}
	return fingerprint.Strings(fields...)
}

func (o OAuthTarget) Matches(server Server) bool {
	return server.Enabled &&
		server.Transport == TransportStreamableHTTP && server.Authorization == "" &&
		o.Equal(OAuthTarget{Authority: server.ReleaseAuthority, Server: server.Name, URL: server.URL, Headers: server.Headers})
}

func (o OAuthTarget) Equal(other OAuthTarget) bool {
	return o.Authority == other.Authority && o.Server == other.Server && o.URL == other.URL && maps.Equal(o.Headers, other.Headers)
}
