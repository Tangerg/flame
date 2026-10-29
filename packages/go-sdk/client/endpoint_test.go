package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestRemoteCallsPreserveEscapedProxyPrefix(t *testing.T) {
	for _, prefix := range []string{"/tenant%2Fname", "/tenant%2F", "/tenant%25name", "/tenant%20name/"} {
		t.Run(prefix, func(t *testing.T) {
			paths := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths <- r.URL.EscapedPath()
				w.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(server.Close)
			client, err := Connect(t.Context(), RemoteConfig{Endpoint: server.URL + prefix})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if _, err := client.Discover(t.Context(), protocol.CallOptions{}); err == nil {
				t.Fatal("missing endpoint was accepted")
			}
			want := prefix
			if want[len(want)-1] == '/' {
				want = want[:len(want)-1]
			}
			want += rpcPath
			select {
			case got := <-paths:
				if got != want {
					t.Fatalf("requested path = %q, want %q", got, want)
				}
			default:
				t.Fatal("request never reached the proxy")
			}
		})
	}
}
