package httporigin

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Endpoint is an absolute HTTP(S) URL with a host, an in-range port and no
// userinfo. It is the single admission rule for configured endpoints; each
// boundary composes its own policy (HTTPS outside loopback, origin-only, no
// query) from the queries below instead of parsing the URL again.
type Endpoint struct {
	url url.URL
	// url.URL drops an empty "#"; admission must still see that it was written.
	fragment bool
}

// ParseEndpoint admits raw as an [Endpoint]. Errors never include raw, which
// may carry credentials.
func ParseEndpoint(raw string) (Endpoint, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		// url.Error retains the complete URL, which may contain credentials.
		if failure, ok := errors.AsType[*url.Error](err); ok {
			err = failure.Err
		}
		return Endpoint{}, fmt.Errorf("parse URL: %w", err)
	}
	endpoint, err := endpointFromURL(parsed)
	if err != nil {
		return Endpoint{}, err
	}
	endpoint.fragment = strings.ContainsRune(raw, '#')
	return endpoint, nil
}

func endpointFromURL(u *url.URL) (Endpoint, error) {
	if u == nil {
		return Endpoint{}, errors.New("URL is nil")
	}
	if u.User != nil {
		return Endpoint{}, errors.New("URL credentials are not allowed")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Endpoint{}, fmt.Errorf("scheme %q is not HTTP or HTTPS", u.Scheme)
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return Endpoint{}, errors.New("an absolute URL with a host is required")
	}
	if port := u.Port(); port != "" {
		if value, err := strconv.ParseUint(port, 10, 16); err != nil || value == 0 {
			return Endpoint{}, errors.New("port must be between 1 and 65535")
		}
	}
	normalized := *u
	normalized.Scheme = scheme
	return Endpoint{url: normalized, fragment: u.Fragment != ""}, nil
}

// URL returns a copy the caller may modify.
func (e Endpoint) URL() *url.URL {
	clone := e.url
	return &clone
}

func (e Endpoint) String() string { return e.url.String() }

func (e Endpoint) Secure() bool { return e.url.Scheme == "https" }

// Loopback reports a host that cannot leave this machine: the localhost name
// or a loopback IP literal.
func (e Endpoint) Loopback() bool {
	return strings.EqualFold(e.url.Hostname(), "localhost") || e.LoopbackIP()
}

// LoopbackIP reports a loopback IP literal; unlike [Endpoint.Loopback] it never
// trusts name resolution.
func (e Endpoint) LoopbackIP() bool {
	address, err := netip.ParseAddr(e.url.Hostname())
	return err == nil && address.Zone() == "" && address.IsLoopback()
}

func (e Endpoint) HasQuery() bool    { return e.url.RawQuery != "" || e.url.ForceQuery }
func (e Endpoint) HasFragment() bool { return e.fragment }

// IsOrigin reports an endpoint that names only its origin: no path beyond
// "/", no query and no fragment.
func (e Endpoint) IsOrigin() bool {
	path := e.url.EscapedPath()
	return (path == "" || path == "/") && !e.HasQuery() && !e.HasFragment()
}

func (e Endpoint) Origin() Origin {
	host := strings.ToLower(e.url.Hostname())
	port := e.url.Port()
	if port == "" {
		if e.Secure() {
			port = "443"
		} else {
			port = "80"
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return Origin{scheme: e.url.Scheme, host: host + ":" + port}
}
