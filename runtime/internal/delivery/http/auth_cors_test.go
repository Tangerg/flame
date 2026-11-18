package http_test

import (
	"bytes"
	netHTTP "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
)

func TestDefaultCORSOriginsAuthorizeShippedDesktopClients(t *testing.T) {
	ts := newGatedServerWithOrigins(t, flamehttp.DefaultCORSOrigins())
	defer ts.Close()

	for _, origin := range []string{
		"wails://localhost",
		"http://wails.localhost",
		"http://127.0.0.1:5174",
		"http://localhost:5173",
	} {
		t.Run(origin, func(t *testing.T) {
			req, err := netHTTP.NewRequest(netHTTP.MethodOptions, ts.URL+"/v2/rpc", nil)
			if err != nil {
				t.Fatalf("new preflight: %v", err)
			}
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", netHTTP.MethodPost)
			req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
			resp, err := netHTTP.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("preflight: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("Allow-Origin = %q, want %q", got, origin)
			}
		})
	}

	if slices.Contains(flamehttp.DefaultCORSOrigins(), "*") {
		t.Fatal("default desktop CORS policy must not allow every browser origin")
	}
	for _, unrelated := range []string{"tauri://localhost", "http://localhost:3000"} {
		if slices.Contains(flamehttp.DefaultCORSOrigins(), unrelated) {
			t.Fatalf("default desktop CORS policy still allows unrelated origin %q", unrelated)
		}
	}
}

func mustLocalToken(t testing.TB, token string) flamehttp.Authentication {
	t.Helper()
	authentication, err := flamehttp.LocalTokenAuthentication(token)
	if err != nil {
		t.Fatal(err)
	}
	return authentication
}

// newGatedServer builds a test server with the local-token gate +
// CORS allowlist set. Token is "test-token", origins is "http://app".
func newGatedServer(t *testing.T) *httptest.Server {
	return newGatedServerWithOrigins(t, []string{"http://app"})
}

func newGatedServerWithOrigins(t *testing.T, origins []string) *httptest.Server {
	t.Helper()
	srv, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint:   newTestEndpoint(t, &fakeRuntime{}, delivery.EndpointConfig{}),
		Addr:       ":0",
		ServerInfo: protocol.ServerInfo{Name: "flame-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID},

		Authentication: mustLocalToken(t, "test-token"),
		CORSOrigins:    origins,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// TestAuthGateMissingToken — gate-on POST without Authorization gets
// 401 + a transport problem. This MUST NOT use the JSON-RPC envelope.
func TestAuthGateMissingToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != netHTTP.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	raw := readBody(resp)
	if !strings.Contains(raw, `"type":"urn:flame:transport:unauthorized"`) {
		t.Fatalf("body = %s, want unauthorized problem", raw)
	}
	if strings.Contains(raw, `"jsonrpc"`) {
		t.Fatalf("401 must be flat JSON, got envelope: %s", raw)
	}
}

func TestAuthGateWrongToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	req.Header.Set("Content-Type", "application/json")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != netHTTP.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestCORSDisallowedOrigin — request from a non-allowlisted origin
// gets no Allow-Origin header (the browser will reject the response).
// We don't 4xx in this case — the request itself is well-formed.
func TestCORSDisallowedOrigin(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestNewServerRefusesAnUngatedNonLoopbackListener(t *testing.T) {
	endpoint := newTestEndpoint(t, &fakeRuntime{}, delivery.EndpointConfig{})
	info := protocol.ServerInfo{Name: "flame-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID}
	for _, addr := range []string{":17171", "0.0.0.0:17171", "192.168.1.10:17171", "localhost:17171", "[::]:17171"} {
		if _, err := flamehttp.NewServer(flamehttp.Config{Endpoint: endpoint, Addr: addr, ServerInfo: info, Authentication: flamehttp.LoopbackOnlyAuthentication()}); err == nil {
			t.Fatalf("NewServer admitted an ungated listener on %q", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		if _, err := flamehttp.NewServer(flamehttp.Config{Endpoint: endpoint, Addr: addr, ServerInfo: info, Authentication: flamehttp.LoopbackOnlyAuthentication()}); err != nil {
			t.Fatalf("NewServer refused loopback listener %q: %v", addr, err)
		}
	}
	if _, err := flamehttp.NewServer(flamehttp.Config{Endpoint: endpoint, Addr: "127.0.0.1:0", ServerInfo: info}); err == nil {
		t.Fatal("NewServer admitted a server without an authentication policy")
	}
	if _, err := flamehttp.LocalTokenAuthentication(""); err == nil {
		t.Fatal("LocalTokenAuthentication admitted an empty token")
	}
}
