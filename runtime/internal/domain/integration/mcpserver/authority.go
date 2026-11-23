package mcpserver

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	framevalue "github.com/Tangerg/frame/value"
)

// Credentials and lifecycle settings are excluded so a token rotation does not
// grant authority to a different endpoint; variable-length Args come last behind
// their count so two descriptors cannot frame to the same sequence.
func (s Server) AuthorityFingerprint() framevalue.Digest {
	authority := s.Source.connector.String()
	if _, release, found := s.Source.Release(); found {
		authority = release.String()
	}
	fields := []string{string(s.Transport()), authority}
	return identity.DigestStrings(append(fields, s.Connection.authorityFields()...)...)
}

func (s Server) SameConnection(other Server) bool {
	return s.Source == other.Source && s.Name == other.Name && s.Connection != nil && s.Connection.sameAs(other.Connection)
}
