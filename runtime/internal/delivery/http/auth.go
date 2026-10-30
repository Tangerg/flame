package http

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
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

// authGate enforces the local-token check on POST /v2/rpc. Two paths
// bypass: the operational sidecars and CORS preflights. Under
// streamable HTTP every stream is a POST, so the gate covers streaming
// too — there is no header-less EventSource to special-case.
//
// On failure, the response is an RFC 9457 application/problem+json 401,
// not a JSON-RPC envelope, because authentication runs below the protocol
// layer.
func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authentication.gated() {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions || isAuthBypassPath(r.URL.Path) || r.URL.Path == PluginControlPath {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, pluginResourcePrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if !validBearer(r.Header.Get(headerAuthorization), s.authentication.token) {
			// RFC 9110 §15.5.2 requires a 401 challenge. The gate uses Bearer.
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(w, problemUnauthorized, "a valid local bearer token is required", true)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAuthBypassPath flags requests that intentionally skip the gate: the
// operational sidecars (no-auth ops endpoints). The RPC endpoint —
// including streaming POSTs — is gated.
func isAuthBypassPath(p string) bool {
	return isPublicEndpointPath(p)
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
