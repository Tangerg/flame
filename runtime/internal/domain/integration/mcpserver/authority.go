package mcpserver

import (
	"maps"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/integrity"
)

// AuthorityFingerprint excludes credentials and connection lifecycle settings:
// a token rotation does not grant authority to a different endpoint. Every slot
// is always present and the variable-length Args come last behind their count,
// so two different descriptors cannot frame to the same field sequence.
func (s Server) AuthorityFingerprint() integrity.Digest {
	var authority string
	if _, release, found := s.Source.Release(); found {
		authority = release.String()
	}
	fields := []string{string(s.Transport), authority}
	if s.Transport == TransportStreamableHTTP {
		return identity.DigestStrings(append(fields, s.URL)...)
	}
	fields = append(fields, s.Command, s.Dir, s.ExecutionEnvironment, strconv.Itoa(len(s.Args)))
	return identity.DigestStrings(append(fields, s.Args...)...)
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
