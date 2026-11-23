package http

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
)

type Authentication struct {
	token string
	open  bool
}

func LocalTokenAuthentication(token string) (Authentication, error) {
	if token == "" {
		return Authentication{}, errors.New("http: local token is required")
	}
	return Authentication{token: token}, nil
}

func LoopbackOnlyAuthentication() Authentication { return Authentication{open: true} }

func (a Authentication) validateListener(addr string) error {
	switch {
	case a.token != "":
		return nil
	case !a.open:
		return errors.New("http: authentication policy is required")
	case !loopbackListener(addr):
		return fmt.Errorf("http: an ungated server must listen on a loopback IP address, not %q", addr)
	default:
		return nil
	}
}

func (a Authentication) gated() bool { return a.token != "" }

func loopbackListener(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) protectRPC(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authentication.gated() {
			if !validBearer(r.Header.Get(headerAuthorization), s.authentication.token) {
				w.Header().Set("WWW-Authenticate", bearerScheme)
				writeProblem(w, problemUnauthorized, "a valid local bearer token is required")
				return
			}
		} else if !s.trustedLoopbackRequest(r) {
			writeProblem(w, problemForbiddenOrigin, "request authority is not trusted by this local server")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) trustedLoopbackRequest(r *http.Request) bool {
	requestOrigin, trusted := localRequestOrigin(r)
	if !trusted {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	u, bare := bareOriginURL(origins[0])
	if !bare {
		return false
	}
	origin, err := httporigin.FromURL(u)
	return (err == nil && origin == requestOrigin) || slices.Contains(s.corsOrigins, origins[0])
}

const (
	wildcardOrigin = "*"
	// Browsers serialize an origin with no host as "null" (RFC 6454).
	opaqueOrigin = "null"
)

func bareOriginURL(value string) (*url.URL, bool) {
	if value == "" || value == wildcardOrigin || value == opaqueOrigin {
		return nil, false
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.ContainsRune(value, '#') {
		return nil, false
	}
	return u, true
}

// The auth-scheme is case-insensitive (RFC 9110 §11.1); the token is compared
// in constant time.
func validBearer(header, expected string) bool {
	scheme, credentials, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return false
	}
	got := strings.TrimSpace(credentials)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func localRequestOrigin(r *http.Request) (httporigin.Origin, bool) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	endpoint, err := httporigin.ParseEndpoint(scheme + "://" + r.Host)
	if err != nil || endpoint.URL().Path != "" || endpoint.HasQuery() || endpoint.HasFragment() || !endpoint.Loopback() {
		return httporigin.Origin{}, false
	}
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		_, actualPort, err := net.SplitHostPort(local.String())
		if err != nil || endpoint.Port() != actualPort {
			return httporigin.Origin{}, false
		}
	}
	return endpoint.Origin(), true
}
