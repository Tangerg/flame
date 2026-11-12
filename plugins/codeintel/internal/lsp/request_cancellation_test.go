package lsp

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

func TestRequestCancellationInterruptsLSPWrites(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(context.Context, *client, string) error
	}{
		{"synchronize document", func(ctx context.Context, c *client, path string) error {
			_, err := c.ensureContent(ctx, path, []byte("package main"))
			return err
		}},
		{"dispatch query", func(ctx context.Context, c *client, _ string) error {
			_, err := c.workspaceSymbols(ctx, "Widget")
			return err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			for _, deadline := range []bool{false, true} {
				name := "cancellation"
				if deadline {
					name = "deadline"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						path := filepath.Join(t.TempDir(), "main.go")
						agent, server := net.Pipe()
						defer agent.Close()
						defer server.Close()
						closeFailure := errors.New("stdio cleanup failed")
						writer := &blockedNotificationConn{Conn: agent, started: make(chan struct{}), closeErr: closeFailure}
						c := &client{
							syncPolicy: documentSynchronization{OpenClose: true, Change: changeFull},
							open:       map[string]openDoc{},
						}
						c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(writer, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
						defer c.conn.Close()
						cause := errors.New("query abandoned")
						ctx, cancel := context.WithCancelCause(t.Context())
						defer cancel(nil)
						expected := context.Canceled
						if deadline {
							var stop context.CancelFunc
							ctx, stop = context.WithTimeout(t.Context(), time.Second)
							defer stop()
							expected = context.DeadlineExceeded
						}
						result := make(chan error, 1)
						go func() { result <- operation.run(ctx, c, path) }()
						<-writer.started
						if deadline {
							time.Sleep(time.Second)
						} else {
							cancel(cause)
						}
						synctest.Wait()
						select {
						case err := <-result:
							if !errors.Is(err, expected) || !errors.Is(err, closeFailure) || !deadline && !errors.Is(err, cause) {
								t.Fatalf("canceled write lost its cancellation or cleanup cause: %v", err)
							}
						default:
							c.conn.Close()
							<-result
							t.Fatal("request cancellation did not release the blocked LSP write")
						}
						select {
						case <-c.conn.DisconnectNotify():
						default:
							t.Fatal("an interrupted LSP frame left the connection reusable")
						}
						if err := operation.run(t.Context(), c, path); !errors.Is(err, jsonrpc2.ErrClosed) {
							t.Fatalf("an interrupted connection accepted another request: %v", err)
						}
					})
				})
			}
		})
	}
}

func TestResponseCancellationKeepsLSPConnectionUsable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		agent, server := net.Pipe()
		defer agent.Close()
		defer server.Close()
		c := &client{}
		c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
		defer c.conn.Close()
		stream := jsonrpc2.NewBufferedStream(server, lspObjectCodec{})
		received := make(chan struct{})
		resume := make(chan struct{})
		release := sync.OnceFunc(func() { close(resume) })
		defer release()
		serverDone := make(chan error, 1)
		go func() {
			for index := range 2 {
				var request jsonrpc2.Request
				if err := stream.ReadObject(&request); err != nil {
					serverDone <- err
					return
				}
				if index == 0 {
					close(received)
					<-resume
				}
				response := &jsonrpc2.Response{ID: request.ID}
				if err := response.SetResult([]Symbol{}); err != nil {
					serverDone <- err
					return
				}
				if err := stream.WriteObject(response); err != nil {
					serverDone <- err
					return
				}
			}
			serverDone <- nil
		}()
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		cause := errors.New("analysis no longer needed")
		result := make(chan error, 1)
		go func() { _, err := c.workspaceSymbols(ctx, "first"); result <- err }()
		<-received
		synctest.Wait()
		cancel(cause)
		synctest.Wait()
		if err := <-result; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("response cancellation = %v", err)
		}
		select {
		case <-c.conn.DisconnectNotify():
			t.Fatal("canceling response observation closed a healthy LSP connection")
		default:
		}
		release()
		if _, err := c.workspaceSymbols(t.Context(), "second"); err != nil {
			t.Fatalf("query after cancellation: %v", err)
		}
		if err := <-serverDone; err != nil {
			t.Fatal(err)
		}
	})
}

func TestAlreadyCanceledRequestDoesNotWriteOrCloseLSPConnection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		agent, server := net.Pipe()
		defer agent.Close()
		defer server.Close()
		writer := &blockedNotificationConn{Conn: agent, started: make(chan struct{})}
		c := &client{}
		c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(writer, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
		defer c.conn.Close()
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("query canceled before dispatch")
		cancel(cause)
		if _, err := c.workspaceSymbols(ctx, "unused"); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("canceled request = %v", err)
		}
		select {
		case <-writer.started:
			t.Fatal("an already canceled request started writing")
		case <-c.conn.DisconnectNotify():
			t.Fatal("an already canceled request closed an unused connection")
		default:
		}
	})
}
