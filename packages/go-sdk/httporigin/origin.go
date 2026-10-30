// Package httporigin owns HTTP(S) origin identity for credential and endpoint
// admission. It has no request or network capability.
package httporigin

import (
	"net/url"
)

// Origin is a normalized, comparable HTTP(S) origin: a lowercased scheme and
// host with the effective default port filled in, so two origins are equal iff
// they are the same security origin. The zero value is not a valid origin.
type Origin struct {
	scheme string // "http" or "https", lowercased
	host   string // host:port, host lowercased, default port materialized
}

// String renders the origin as "scheme://host:port".
func (o Origin) String() string { return o.scheme + "://" + o.host }

// Parse admits raw as an [Endpoint] and returns its origin.
func Parse(raw string) (Origin, error) {
	endpoint, err := ParseEndpoint(raw)
	if err != nil {
		return Origin{}, err
	}
	return endpoint.Origin(), nil
}

// FromURL returns the origin of an already-parsed URL (the redirect-target
// path, where the URL is in hand). A nil URL fails closed.
func FromURL(u *url.URL) (Origin, error) {
	endpoint, err := endpointFromURL(u)
	if err != nil {
		return Origin{}, err
	}
	return endpoint.Origin(), nil
}

// Same reports whether two endpoints share the same origin. Invalid or
// non-HTTP(S) endpoints never match (fail-closed).
func Same(left, right string) bool {
	leftOrigin, err := Parse(left)
	if err != nil {
		return false
	}
	rightOrigin, err := Parse(right)
	return err == nil && leftOrigin == rightOrigin
}
