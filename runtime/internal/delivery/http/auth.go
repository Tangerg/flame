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
)

// Authentication is the RPC gate policy. The zero value is invalid so every
// server chooses one, and an ungated server is admitted only on a loopback
// listener: the RPC surface runs shell and file tools.
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

// A loopback listener establishes the socket peer, not the browser origin.
// In particular a rebinding domain can be same-origin to the browser.
func (s *Server) protectRPC(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authentication.gated() {
			if !validBearer(r.Header.Get(headerAuthorization), s.authentication.token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeProblem(w, problemUnauthorized, "a valid local bearer token is required", true)
				return
			}
		} else if !s.trustedLoopbackRequest(r) {
			writeProblem(w, problemForbiddenOrigin, "request authority is not trusted by this local server", true)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) trustedLoopbackRequest(r *http.Request) bool {
	if !localRequestAuthority(r) {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 || origins[0] == "" || origins[0] == "*" || origins[0] == "null" {
		return false
	}
	u, err := url.Parse(origins[0])
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return (u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)) || slices.Contains(s.corsOrigins, origins[0])
}

// The auth-scheme is case-insensitive (RFC 9110 §11.1); the token is compared
// in constant time.
func validBearer(header, expected string) bool {
	scheme, credentials, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	got := strings.TrimSpace(credentials)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func localRequestAuthority(r *http.Request) bool {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// HTTP omits its default port in Host and Origin.
		u := &url.URL{Host: r.Host}
		host = u.Hostname()
		if r.Host != host && r.Host != "["+host+"]" {
			return false
		}
		port = "80"
		if r.TLS != nil {
			port = "443"
		}
	}
	if !(strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()) {
		return false
	}
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		_, actualPort, err := net.SplitHostPort(local.String())
		if err != nil || port != actualPort {
			return false
		}
	}
	return true
}
