package mcpserver

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

var ErrOAuthSessionSuperseded = errors.New("mcp: oauth session superseded")

// OAuthTarget identifies the configuration that requested a credential grant.
// It is the only owner of credential validity: a grant is bound to its
// target's fingerprint and is usable only by a configuration with the same one.
type OAuthTarget struct {
	Source  Source
	Name    ServerName
	URL     string
	Headers map[string]string
}

func (o OAuthTarget) ID() ID { return o.Source.ID(o.Name) }

// Fingerprint binds a persisted grant to the identity that requested it and
// the recipient its credential reaches. A user record's recipient is its
// complete endpoint configuration, including headers. An installation record's
// recipient is declared by its source, which excludes configured input values
// and survives a release that keeps the endpoint, so an OAuth grant and a
// static secret input follow the same rule.
func (o OAuthTarget) Fingerprint() fingerprint.Digest {
	installation, installed := o.Source.Origin().Installation()
	fields := []string{string(o.Source.Origin().Kind()), installation.String(), o.Name.String()}
	if installed {
		return fingerprint.Strings(append(fields, o.Source.recipient.String())...)
	}
	fields = append(fields, o.URL)
	for _, name := range slices.Sorted(maps.Keys(o.Headers)) {
		fields = append(fields, name, o.Headers[name])
	}
	return fingerprint.Strings(fields...)
}

// OAuthTarget is the credential target this record would request a grant for.
func (s Server) OAuthTarget() OAuthTarget {
	return OAuthTarget{Source: s.Source, Name: s.Name, URL: s.URL, Headers: maps.Clone(s.Headers)}
}
