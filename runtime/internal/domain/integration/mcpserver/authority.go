package mcpserver

import (
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// AuthorityFingerprint excludes credentials and connection lifecycle settings:
// a token rotation does not grant authority to a different endpoint.
func (s Server) AuthorityFingerprint() identity.Digest {
	fields := []string{string(s.Transport)}
	if _, authority, found := s.Source.Release(); found {
		fields = append(fields, authority.String())
	}
	if s.Transport == TransportStreamableHTTP {
		fields = append(fields, s.URL)
	} else {
		fields = append(fields, s.Command, s.Dir)
		fields = append(fields, s.Args...)
		if s.ExecutionEnvironment != "" {
			fields = append(fields, s.ExecutionEnvironment)
		}
	}
	return identity.DigestStrings(fields...)
}

// SameConnection compares the descriptor realized by an executable. Enablement
// is checked separately at dispatch; notes and future handshake limits cannot
// change an existing connection. An absent process environment inherits the
// parent, whereas an explicitly empty environment does not.
func (s Server) SameConnection(other Server) bool {
	if s.Source != other.Source || s.Name != other.Name || s.Transport != other.Transport {
		return false
	}
	switch s.Transport {
	case TransportStreamableHTTP:
		return s.URL == other.URL && s.Authorization == other.Authorization && maps.Equal(s.Headers, other.Headers)
	case TransportStdio:
		return s.Command == other.Command && s.ExecutionEnvironment == other.ExecutionEnvironment && s.Dir == other.Dir && slices.Equal(s.Args, other.Args) &&
			(s.Env == nil) == (other.Env == nil) && maps.Equal(s.Env, other.Env)
	default:
		return false
	}
}
