package lsp

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

type rpcHandler func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request)

func (h rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	h(ctx, conn, request)
}

func connectedTestClient(t *testing.T, handler rpcHandler) (*client, *jsonrpc2.Conn) {
	t.Helper()
	agent, server := net.Pipe()
	wait := make(chan error)
	close(wait)
	c := &client{
		spec: ServerSpec{Name: "test", LanguageID: "go"}, shutdownBase: context.WithoutCancel(t.Context()),
		cancel: func() {}, wait: wait, stopProcess: func() error { return nil },
		open: make(map[string]openDoc), diags: make(map[string]diagSet), updated: make(chan struct{}),
	}
	remote := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(server, lspObjectCodec{}), handler)
	c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
	t.Cleanup(func() { _ = c.close(); _ = remote.Close() })
	return c, remote
}

func replyEmpty(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	if !request.Notif {
		_ = conn.Reply(ctx, request.ID, nil)
	}
}

func TestDiagnosticsRequiresTheSyncedDocumentVersion(t *testing.T) {
	for _, prior := range []bool{false, true} {
		t.Run(map[bool]string{false: "no observation", true: "stale observation"}[prior], func(t *testing.T) {
			c, _ := connectedTestClient(t, replyEmpty)
			path := filepath.Join(t.TempDir(), "main.go")
			if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			version, err := c.ensureOpen(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if prior {
				c.storeDiagnostics(publishDiagnosticsParams{URI: pathToURI(path), Version: version, Diagnostics: []Diagnostic{{Message: "old problem"}}})
				if err := os.WriteFile(path, []byte("package changed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			diagnostics, err := c.diagnostics(t.Context(), path, 0)
			pending, isPending := errors.AsType[*DiagnosticsPendingError](err)
			if !isPending || len(diagnostics) != 0 || (pending.ObservedVersion != nil) != prior {
				t.Fatalf("unobserved current version = %+v, %v; want pending", diagnostics, err)
			}
		})
	}
}

func TestDiagnosticsWaitPreservesCancellationAndDisconnect(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation", true: "disconnect"}[disconnect], func(t *testing.T) {
			saved := make(chan struct{}, 1)
			c, remote := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
				if request.Method == "textDocument/didSave" {
					saved <- struct{}{}
				}
				replyEmpty(ctx, conn, request)
			})
			path := filepath.Join(t.TempDir(), "main.go")
			if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cause := errors.New("observation withdrawn")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			done := make(chan error, 1)
			go func() { _, err := c.diagnostics(ctx, path, time.Hour); done <- err }()
			<-saved
			want := cause
			if disconnect {
				want = jsonrpc2.ErrClosed
				_ = remote.Close()
			} else {
				cancel(cause)
			}
			if err := <-done; !errors.Is(err, want) || !disconnect && !errors.Is(err, context.Canceled) {
				t.Fatalf("diagnostics wait = %v, want %v", err, want)
			}
		})
	}
}

func TestDiagnosticsAcceptsDelayedExplicitEmptyAndRejectsVersionRegression(t *testing.T) {
	saved := make(chan struct{}, 1)
	c, remote := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
		if request.Method == "textDocument/didSave" {
			saved <- struct{}{}
		}
		replyEmpty(ctx, conn, request)
	})
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	notified := make(chan error, 1)
	go func() {
		<-saved
		notified <- remote.Notify(t.Context(), "textDocument/publishDiagnostics", publishDiagnosticsParams{
			URI: pathToURI(path), Version: 1, Diagnostics: []Diagnostic{},
		})
	}()
	diagnostics, err := c.diagnostics(t.Context(), path, time.Hour)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("fresh empty = %+v, %v", diagnostics, err)
	}
	if err := <-notified; err != nil {
		t.Fatal(err)
	}
	c.storeDiagnostics(publishDiagnosticsParams{URI: pathToURI(path), Version: 1, Diagnostics: []Diagnostic{{Message: "current"}}})
	c.storeDiagnostics(publishDiagnosticsParams{URI: pathToURI(path), Version: 0, Diagnostics: []Diagnostic{}})
	diagnostics, err = c.diagnostics(t.Context(), path, 0)
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Message != "current" {
		t.Fatalf("older push replaced current diagnostics: %+v, %v", diagnostics, err)
	}
}

func TestWorkspaceSymbolsPreservesPartialFailure(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "empty success", false: "nonempty success"}[empty], func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "marker"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			servers := mustNewServers(t, []ServerSpec{
				{Name: "go", Command: "unused", Extensions: []string{".go"}, LanguageID: "go", RootMarkers: []string{"marker"}},
				{Name: "typescript", Command: "unused", Extensions: []string{".ts"}, LanguageID: "typescript", RootMarkers: []string{"marker"}},
			})
			c, _ := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
				if request.Method == "workspace/symbol" {
					result := []symbolInformation{}
					if !empty {
						result = append(result, symbolInformation{Name: "Found", Kind: 12, Location: &Location{URI: "file:///main.go"}})
					}
					_ = conn.Reply(ctx, request.ID, result)
					return
				}
				replyEmpty(ctx, conn, request)
			})
			failure := errors.New("server unavailable")
			servers.launch = func(_ context.Context, spec ServerSpec, _ string) (*client, error) {
				if spec.Name == "go" {
					return c, nil
				}
				return nil, failure
			}
			symbols, err := servers.WorkspaceSymbols(t.Context(), root, "Found")
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), "typescript") || !empty && len(symbols) != 1 {
				t.Fatalf("partial search = %+v, %v", symbols, err)
			}
			cause := errors.New("symbol observation withdrawn")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			servers.launch = func(context.Context, ServerSpec, string) (*client, error) {
				cancel(cause)
				return nil, failure
			}
			symbols, err = servers.WorkspaceSymbols(ctx, root, "Found")
			if len(symbols) != 0 || !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled partial search = %+v, %v", symbols, err)
			}
		})
	}
}
