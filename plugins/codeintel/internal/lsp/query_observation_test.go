package lsp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
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
	wait := make(chan processEnd)
	close(wait)
	c := &client{
		workspace: testWorkspace(t, t.TempDir()), syncPolicy: documentSynchronization{OpenClose: true, Change: changeFull, Save: saveSynchronization{Enabled: true}}, spec: ServerSpec{Name: "test", Languages: map[string]string{".go": "go"}}, shutdownBase: context.WithoutCancel(t.Context()),
		cancel: func() {}, wait: wait, stopProcess: func() error { return nil },
		open: make(map[string]openDoc), diags: make(map[string]diagSet), updated: make(chan struct{}),
	}
	remote := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(server, lspObjectCodec{}), handler)
	c.conn = newConnection(t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
	t.Cleanup(func() { _ = c.close(); _ = c.conn.Close(); _ = remote.Close() })
	return c, remote
}

func replyEmpty(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	if !request.Notif {
		_ = conn.Reply(ctx, request.ID, nil)
	}
}

func TestInitializeAdmitsOnlyUTF16PositionEncoding(t *testing.T) {
	for _, encoding := range []string{"", "utf-16", "utf-8", "utf-32"} {
		t.Run("encoding="+encoding, func(t *testing.T) {
			initialized := make(chan struct{}, 1)
			c, _ := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
				switch request.Method {
				case "initialize":
					var params initializeParams
					if err := json.Unmarshal(*request.Params, &params); err != nil {
						t.Error(err)
					}
					if !slices.Equal(params.Capabilities.General.PositionEncodings, []string{"utf-16"}) {
						t.Errorf("advertised encodings: %+v", params.Capabilities.General.PositionEncodings)
					}
					capabilities := map[string]any{"textDocumentSync": 2}
					if encoding != "" {
						capabilities["positionEncoding"] = encoding
					}
					_ = conn.Reply(ctx, request.ID, map[string]any{"capabilities": capabilities})
				case "initialized":
					initialized <- struct{}{}
				default:
					replyEmpty(ctx, conn, request)
				}
			})
			err := c.initialize(t.Context())
			if encoding == "" || encoding == "utf-16" {
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-initialized:
				case <-time.After(time.Second):
					t.Fatal("admitted encoding did not complete initialization")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "position encoding") {
					t.Fatalf("unsupported encoding: %v", err)
				}
				select {
				case <-initialized:
					t.Fatal("unsupported encoding completed initialization")
				default:
				}
			}
		})
	}
}

func TestDiagnosticsRequiresTheSyncedDocumentVersion(t *testing.T) {
	for _, prior := range []bool{false, true} {
		t.Run(map[bool]string{false: "no observation", true: "stale observation"}[prior], func(t *testing.T) {
			c, _ := connectedTestClient(t, replyEmpty)
			path := filepath.Join(c.workspace.Name(), "main.go")
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
			path := filepath.Join(c.workspace.Name(), "main.go")
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
	path := filepath.Join(c.workspace.Name(), "main.go")
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
				{Name: "go", Command: "unused", Languages: map[string]string{".go": "go"}, RootMarkers: []string{"marker"}},
				{Name: "typescript", Command: "unused", Languages: map[string]string{".ts": "typescript"}, RootMarkers: []string{"marker"}},
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

func TestSaveIncludesExactCapturedEmptyTextWhenServerRequestsIt(t *testing.T) {
	saved := make(chan didSaveParams, 1)
	c, _ := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		if req.Method == "textDocument/didSave" {
			var params didSaveParams
			if err := json.Unmarshal(*req.Params, &params); err != nil {
				t.Error(err)
			}
			saved <- params
		}
		replyEmpty(ctx, conn, req)
	})
	options, err := negotiatedSave(jsontext.Value(`{"save":{"includeText":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	c.syncPolicy.Save = options
	path := filepath.Join(t.TempDir(), "unwritten.go")
	_, err = c.diagnosticsContent(t.Context(), path, []byte{}, 0)
	if _, ok := errors.AsType[*DiagnosticsPendingError](err); !ok {
		t.Fatalf("diagnostics: %v", err)
	}
	select {
	case params := <-saved:
		if params.Text == nil || *params.Text != "" || params.TextDocument.URI != pathToURI(path) {
			t.Fatalf("incorrect didSave payload: %+v", params)
		}
	case <-time.After(time.Second):
		t.Fatal("didSave was not delivered")
	}
}

func TestCallHierarchyIncludesEveryPreparedRootAndPreservesServerData(t *testing.T) {
	c, _ := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
		switch req.Method {
		case "textDocument/prepareCallHierarchy":
			_ = conn.Reply(ctx, req.ID, []map[string]string{{"name": "first", "data": "one"}, {"name": "second", "data": "two"}})
		case "callHierarchy/outgoingCalls":
			var params struct {
				Item struct {
					Data string `json:"data"`
				} `json:"item"`
			}
			if err := json.Unmarshal(*req.Params, &params); err != nil {
				t.Error(err)
			}
			_ = conn.Reply(ctx, req.ID, []callHierarchyOutgoingCall{{To: callHierarchyItem{Name: params.Item.Data, URI: "file:///root.go"}}})
		default:
			replyEmpty(ctx, conn, req)
		}
	})
	path := filepath.Join(c.workspace.Name(), "source.go")
	if err := os.WriteFile(path, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := c.callHierarchy(t.Context(), path, Position{}, true)
	if err != nil || len(result) != 2 || result[0].Name != "one" || result[1].Name != "two" {
		t.Fatalf("roots = %+v, %v", result, err)
	}
}

func TestWorkspaceFolderReplyAndUnsupportedServerMethod(t *testing.T) {
	client, remote := connectedTestClient(t, replyEmpty)
	var folders []workspaceFolder
	if err := remote.Call(t.Context(), "workspace/workspaceFolders", nil, &folders); err != nil || len(folders) != 1 || folders[0].URI != pathToURI(client.workspace.Name()) {
		t.Fatalf("folders = %+v, %v", folders, err)
	}
	err := remote.Call(t.Context(), "workspace/notSupported", nil, nil)
	var problem *jsonrpc2.Error
	if !errors.As(err, &problem) || problem.Code != jsonrpc2.CodeMethodNotFound {
		t.Fatalf("unknown method: %v", err)
	}
}

func TestIncrementalChangeReplacesPreviousUTF16Range(t *testing.T) {
	changes := make(chan didChangeParams, 1)
	client, _ := connectedTestClient(t, func(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
		if request.Method == "textDocument/didChange" {
			var params didChangeParams
			if err := json.Unmarshal(*request.Params, &params); err != nil {
				t.Error(err)
			}
			changes <- params
		}
		replyEmpty(ctx, conn, request)
	})
	client.syncPolicy.Change = changeIncremental
	path := filepath.Join(client.workspace.Name(), "view.go")
	if _, err := client.ensureContent(t.Context(), path, []byte("a\r\n😀!")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ensureContent(t.Context(), path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	select {
	case params := <-changes:
		change := params.ContentChanges[0]
		if change.Range == nil || *change.Range != (Range{End: Position{Line: 1, Character: 3}}) || change.Text != "new" {
			t.Fatalf("wrong replacement: %+v", change)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing document change")
	}
}
