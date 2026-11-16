package http

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopbackRPCRequiresTrustedBrowserAuthority(t *testing.T) {
	for _, tc := range []struct {
		host, origin string
		accepted     bool
	}{
		{"attacker.test:17171", "http://attacker.test:17171", false},
		{"127.0.0.1:17171", "http://attacker.test:17171", false},
		{"127.0.0.1:17171", "null", false},
		{"127.0.0.1:17171", "", true},
		{"127.0.0.1:17171", "http://127.0.0.1:17171", true},
		{"127.0.0.1:80", "http://127.0.0.1", true},
		{"[0:0:0:0:0:0:0:1]:17171", "http://[::1]:17171", true},
		{"127.0.0.1:17171", "http://127.0.0.1:17171?", false},
		{"127.0.0.1:17171", "http://127.0.0.1:17171#", false},
		{"127.0.0.1:17171", "wails://localhost", true},
	} {
		s := &Server{authentication: LoopbackOnlyAuthentication(), corsOrigins: []string{"wails://localhost", "*"}}
		r := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/v2/rpc", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		called := false
		s.protectRPC(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(httptest.NewRecorder(), r)
		if called != tc.accepted {
			t.Errorf("host=%s origin=%s accepted=%t", tc.host, tc.origin, called)
		}
	}
}

func TestLoopbackRequestUsesTheSocketPort(t *testing.T) {
	for _, tc := range []struct {
		host   string
		secure bool
		port   int
		want   bool
	}{
		{"localhost:017171", false, 17171, true},
		{"localhost", false, 80, true},
		{"localhost", true, 443, true},
		{"localhost", false, 17171, false},
		{"localhost:17172", false, 17171, false},
		{"localhost:0", false, 17171, false},
		{"localhost:65536", false, 17171, false},
		{"localhost:17171/", false, 17171, false},
		{"localhost:17171?", false, 17171, false},
		{"localhost:17171#", false, 17171, false},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost/v2/rpc", nil)
		r.Host = tc.host
		if tc.secure {
			r.TLS = &tls.ConnectionState{}
		}
		r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: tc.port}))
		if _, trusted := localRequestOrigin(r); trusted != tc.want {
			t.Errorf("host=%s secure=%v port=%d trusted=%v, want %v", tc.host, tc.secure, tc.port, trusted, tc.want)
		}
	}
}
