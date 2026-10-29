// Package httporigin owns HTTP(S) origin identity for credential and endpoint
// admission. It has no request or network capability.
package httporigin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
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

// Parse normalizes a raw HTTP(S) URL into an [Origin], failing closed on an
// unparseable URL, a non-HTTP(S) scheme, or a missing host.
func Parse(raw string) (Origin, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error retains the complete URL, which may contain credentials.
		// Keep the syntax cause without carrying that decorated input forward.
		if failure, ok := errors.AsType[*url.Error](err); ok {
			err = failure.Err
		}
		return Origin{}, fmt.Errorf("parse URL: %w", err)
	}
	return FromURL(u)
}

// FromURL normalizes an already-parsed HTTP(S) URL into an [Origin] (the
// redirect-target path, where the URL is in hand). A nil URL fails closed.
func FromURL(u *url.URL) (Origin, error) {
	if u == nil {
		return Origin{}, errors.New("URL is nil")
	}
	if u.User != nil {
		return Origin{}, errors.New("URL credentials are not allowed")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Origin{}, fmt.Errorf("scheme %q is not HTTP or HTTPS", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return Origin{}, errors.New("host is required")
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return Origin{scheme: scheme, host: host + ":" + port}, nil
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
