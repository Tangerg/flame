package plugin

import (
	"testing"

	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newTestCallbacks(t *testing.T) *qimumcp.Callbacks {
	t.Helper()
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := callbacks.Close(); err != nil {
			t.Error(err)
		}
	})
	return callbacks
}

var callbackTestLimits = qimumcp.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}

func connectMethodSessions(t *testing.T, server *sdk.Server, client *sdk.Client) *sdk.ClientSession {
	t.Helper()
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func connectPublishedBackend(t *testing.T, backend *qimumcp.Backend, server *sdk.Server, client *sdk.Client) *sdk.ClientSession {
	t.Helper()
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	return connectMethodSessions(t, server, client)
}

func testBackend(t *testing.T, server *sdk.Server) *qimumcp.Backend {
	t.Helper()
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func testMethod[P, R any](t *testing.T, name string, kind portable.MethodKind) *qimumcp.Method[P, R] {
	t.Helper()
	method, err := qimumcp.NewMethod[P, R](qimumcp.MethodConfig{Name: name, Kind: kind, Target: "installation", Description: name})
	if err != nil {
		t.Fatal(err)
	}
	return method
}
