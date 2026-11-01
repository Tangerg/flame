package http

import (
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
			t.Fatalf("host=%s origin=%s accepted=%t", tc.host, tc.origin, called)
		}
	}
}
