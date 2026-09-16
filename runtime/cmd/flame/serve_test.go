package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/localruntime"
)

type scriptedHTTPServer struct {
	start    func() error
	shutdown func(context.Context) error
	close    func() error
}

func (s scriptedHTTPServer) Start() error { return s.start() }

func (s scriptedHTTPServer) Shutdown(ctx context.Context) error { return s.shutdown(ctx) }

func (s scriptedHTTPServer) Close() error { return s.close() }

type scriptedRuntimeCloser struct {
	results []error
	closes  int
}

func (s *scriptedRuntimeCloser) Close() error {
	result := s.results[s.closes]
	s.closes++
	return result
}

func TestResolvedVersionPrefersExplicitLinkValue(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })

	if got := resolvedVersion(); got != "v1.2.3" {
		t.Fatalf("resolvedVersion = %q, want explicit link value", got)
	}
}

func TestCloseRuntimeInstanceRetriesIncompleteShutdown(t *testing.T) {
	transient := errors.New("component still draining")
	instance := &scriptedRuntimeCloser{results: []error{transient, transient, nil}}
	if err := closeRuntimeInstance(instance); err != nil {
		t.Fatalf("closeRuntimeInstance: %v", err)
	}
	if instance.closes != 3 {
		t.Fatalf("runtime close attempts = %d, want 3", instance.closes)
	}
}

func TestCloseRuntimeInstanceBoundsRepeatedFailure(t *testing.T) {
	want := errors.New("component shutdown failed")
	instance := &scriptedRuntimeCloser{results: []error{want, want, want, nil}}
	err := closeRuntimeInstance(instance)
	if !errors.Is(err, want) || instance.closes != runtimeCloseAttempts {
		t.Fatalf("close result = (%v, %d attempts)", err, instance.closes)
	}
}

func TestRunServerClosesTransportAfterServeFailure(t *testing.T) {
	serveFailure := errors.New("serve failed")
	closeFailure := errors.New("close failed")
	closed := 0
	server := scriptedHTTPServer{
		start:    func() error { return serveFailure },
		shutdown: func(context.Context) error { t.Fatal("unexpected graceful shutdown"); return nil },
		close: func() error {
			closed++
			return closeFailure
		},
	}

	err := runServer(t.Context(), io.Discard, server, "127.0.0.1:0", nil)
	if !errors.Is(err, serveFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("runServer error = %v, want serve and close failures", err)
	}
	if closed != 1 {
		t.Fatalf("transport close calls = %d, want 1", closed)
	}
}

func TestRunServerUsesGracefulShutdownAfterOwnerCancellation(t *testing.T) {
	started := make(chan struct{})
	serveDone := make(chan struct{})
	shutdowns := 0
	closes := 0
	server := scriptedHTTPServer{
		start: func() error {
			close(started)
			<-serveDone
			return http.ErrServerClosed
		},
		shutdown: func(context.Context) error {
			shutdowns++
			close(serveDone)
			return nil
		},
		close: func() error {
			closes++
			return nil
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- runServer(ctx, io.Discard, server, "127.0.0.1:0", nil) }()
	<-started
	cancel()

	if err := <-result; err != nil {
		t.Fatalf("runServer: %v", err)
	}
	if shutdowns != 1 || closes != 0 {
		t.Fatalf("transport cleanup = (%d shutdowns, %d closes), want (1, 0)", shutdowns, closes)
	}
}

// TestBannerReportsTheGateThatActuallyExists pins the one line an operator
// reads to learn whether the RPC surface is protected. The registry declares
// that /v2/rpc intends to be token-gated; only a configured token supplies the
// gate. Printing the declaration alone put "token-gated" beside an open
// endpoint on a runtime started with server.noLocalToken.
func TestBannerReportsTheGateThatActuallyExists(t *testing.T) {
	banner := func(token *localruntime.Token) string {
		t.Helper()
		var printed bytes.Buffer
		server := scriptedHTTPServer{
			start:    func() error { return errors.New("stop after the banner") },
			shutdown: func(context.Context) error { return nil },
			close:    func() error { return nil },
		}
		_ = runServer(t.Context(), &printed, server, "127.0.0.1:0", token)
		return printed.String()
	}

	open := banner(nil)
	if !strings.Contains(open, "local-token gate disabled") {
		t.Fatalf("banner = %q, want the disabled gate reported", open)
	}
	for _, line := range strings.Split(open, "\n") {
		if strings.Contains(line, "/v2/rpc") && strings.Contains(line, "token-gated") {
			t.Fatalf("an ungated RPC endpoint is printed as token-gated: %q", line)
		}
	}

	token, err := localruntime.OpenToken(filepath.Join(t.TempDir(), "token"))
	if err != nil {
		t.Fatal(err)
	}
	gated := banner(token)
	if !strings.Contains(gated, "local-token gate active") {
		t.Fatalf("banner = %q, want the active gate reported", gated)
	}
	rpcGated := false
	for _, line := range strings.Split(gated, "\n") {
		if strings.Contains(line, "/v2/rpc") && strings.Contains(line, "token-gated") {
			rpcGated = true
		}
	}
	if !rpcGated {
		t.Fatalf("banner = %q, want the gated RPC endpoint named token-gated", gated)
	}
}
