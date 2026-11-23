package httporigin

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

const (
	schemeHTTP       = "http"
	schemeHTTPS      = "https"
	defaultHTTPPort  = "80"
	defaultHTTPSPort = "443"
	localhostName    = "localhost"
)

type Endpoint struct {
	url      url.URL
	fragment bool
}

// Errors never include raw, which may carry credentials.
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
	if scheme != schemeHTTP && scheme != schemeHTTPS {
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
	host, err := canonicalHost(u.Hostname())
	if err != nil {
		return Endpoint{}, err
	}
	if host == "" {
		return Endpoint{}, errors.New("an absolute URL with a host is required")
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	normalized := *u
	normalized.Scheme = scheme
	normalized.Host = host
	return Endpoint{url: normalized, fragment: u.Fragment != ""}, nil
}

func canonicalHost(host string) (string, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String(), nil
	}
	// Match net/http's ASCII admission; IDNA would reject existing internal
	// names with underscores even though the transport accepts them.
	if strings.ContainsFunc(host, func(char rune) bool { return char > unicode.MaxASCII }) {
		ascii, err := idna.Lookup.ToASCII(host)
		if err != nil {
			return "", fmt.Errorf("hostname: %w", err)
		}
		host = ascii
	}
	return strings.ToLower(host), nil
}

func (e Endpoint) URL() *url.URL {
	clone := e.url
	return &clone
}

// Escaped path separators stay distinct because proxies may route them
// differently.
func (e Endpoint) CanonicalBase() string {
	origin := e.Origin()
	path := e.url.JoinPath()
	base := url.URL{Scheme: origin.scheme, Host: origin.host, Path: path.Path, RawPath: path.RawPath}
	return strings.TrimRight(base.String(), "/")
}

func (e Endpoint) String() string { return e.url.String() }

func (e Endpoint) Secure() bool { return e.url.Scheme == schemeHTTPS }

func (e Endpoint) Loopback() bool {
	return e.url.Hostname() == localhostName || e.LoopbackIP()
}

func (e Endpoint) LoopbackIP() bool {
	address, err := netip.ParseAddr(e.url.Hostname())
	return err == nil && address.Zone() == "" && address.IsLoopback()
}

func (e Endpoint) HasQuery() bool    { return e.url.RawQuery != "" || e.url.ForceQuery }
func (e Endpoint) HasFragment() bool { return e.fragment }

func (e Endpoint) Port() string {
	if port := strings.TrimLeft(e.url.Port(), "0"); port != "" {
		return port
	}
	if e.Secure() {
		return defaultHTTPSPort
	}
	return defaultHTTPPort
}

func (e Endpoint) IsOrigin() bool {
	path := e.url.EscapedPath()
	return (path == "" || path == "/") && !e.HasQuery() && !e.HasFragment()
}

func (e Endpoint) Origin() Origin {
	host := e.url.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return Origin{scheme: e.url.Scheme, host: host + ":" + e.Port()}
}
