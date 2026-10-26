package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNewOAuthFlowHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	flow, err := newOAuthFlow(ctx)
	if flow != nil {
		if closeErr := flow.close(t.Context()); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("newOAuthFlow error = %v, want context.Canceled", err)
	}
}

func TestOAuthFlowCloseSettlesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	flow, err := newOAuthFlow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flow.server.Close() })
	cancel()
	if err := flow.close(ctx); err != nil {
		t.Fatalf("close after cancellation: %v", err)
	}
	endpoint, err := url.Parse(flow.redirectURI)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", endpoint.Host)
	if err == nil {
		_ = connection.Close()
		t.Fatal("callback server still accepts connections after close")
	}
}

func TestOAuthFlowCloseReportsForcedShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct{})
	flow := &oauthFlow{
		server: &http.Server{
			ConnState: func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					close(accepted)
				}
			},
		},
		serveDone: make(chan error, 1),
	}
	t.Cleanup(func() { _ = flow.server.Close() })
	go func() { flow.serveDone <- flow.server.Serve(listener) }()
	connection, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("callback server did not accept connection")
	}
	// A browser can preconnect without sending a request. Graceful shutdown
	// must time out and retire that connection before reporting its failure.
	if err := flow.close(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close with unfinished connection = %v, want deadline exceeded", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("callback connection survived forced close: %v", err)
	}
}
