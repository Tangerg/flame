package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
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
