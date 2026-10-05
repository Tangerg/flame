package mcpserver

import (
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// AuthorityFingerprint excludes credentials and connection lifecycle settings:
// a token rotation does not grant authority to a different endpoint.
func (s Server) AuthorityFingerprint() fingerprint.Digest {
	fields := []string{string(s.Transport)}
	if _, authority, found := s.Source.Release(); found {
		fields = append(fields, authority.String())
	}
	if s.Transport == TransportStreamableHTTP {
		fields = append(fields, s.URL)
	} else {
		fields = append(fields, s.Command, s.Dir)
		fields = append(fields, s.Args...)
	}
	return fingerprint.Strings(fields...)
}
