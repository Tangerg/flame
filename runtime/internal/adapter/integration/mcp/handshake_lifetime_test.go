package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestHandshakeRejectsCanceledAdmissionBeforeConnect(t *testing.T) {
	cause := errors.New("handshake owner stopped")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	for _, lifetime := range []struct {
		name          string
		parent, owner context.Context
	}{
		{"handshake", ctx, t.Context()},
		{"session owner", t.Context(), ctx},
	} {
		t.Run(lifetime.name, func(t *testing.T) {
			session, release, err := connectSession(lifetime.parent, lifetime.owner, nil, func(context.Context) (*sdkmcp.ClientSession, error) {
				t.Error("canceled admission started connection I/O")
				return nil, errors.New("unexpected connection")
			})
			if release != nil {
				t.Cleanup(release)
			}
			if session != nil || release != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("handshake = (%v, %v, %v), want rejected lifetime and cause", session, release != nil, err)
			}
		})
	}
}

func TestHandshakeFailureRetainsParentCancellation(t *testing.T) {
	cause := errors.New("handshake request stopped")
	failure := errors.New("transport failed")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	_, _, err := connectSession(ctx, t.Context(), nil, func(context.Context) (*sdkmcp.ClientSession, error) {
		cancel(cause)
		return nil, failure
	})
	if !errors.Is(err, failure) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("handshake = %v, want transport failure, cancellation and cause", err)
	}
}

func TestHandshakeFailureRetainsOwnerCancellation(t *testing.T) {
	cause := errors.New("session owner stopped")
	failure := errors.New("transport failed")
	owner, stopOwner := context.WithCancelCause(t.Context())
	defer stopOwner(nil)
	_, _, err := connectSession(t.Context(), owner, nil, func(context.Context) (*sdkmcp.ClientSession, error) {
		stopOwner(cause)
		return nil, failure
	})
	if !errors.Is(err, failure) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("handshake = %v, want transport failure, owner cancellation and cause", err)
	}
}

func TestHandshakeFailureRetainsTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("transport failed")
		timeout := time.Second
		_, _, err := connectSession(t.Context(), t.Context(), &timeout, func(ctx context.Context) (*sdkmcp.ClientSession, error) {
			<-ctx.Done()
			return nil, failure
		})
		if !errors.Is(err, failure) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("handshake = %v, want transport failure and timeout", err)
		}
	})
}

func TestHandshakeCleanupDoesNotConvertTransportFailureIntoCancellation(t *testing.T) {
	failure := errors.New("transport failed")
	var lifetime context.Context
	session, release, err := connectSession(t.Context(), t.Context(), nil, func(ctx context.Context) (*sdkmcp.ClientSession, error) {
		lifetime = ctx
		return nil, failure
	})
	if session != nil || release != nil || !errors.Is(err, failure) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handshake = (%v, %v, %v), want original transport failure", session, release != nil, err)
	}
	if lifetime.Err() != context.Canceled {
		t.Error("failed handshake retained its untransferred session lifetime")
	}
}

func TestHandshakePanicReleasesUntransferredLifetime(t *testing.T) {
	failure := errors.New("connector panicked")
	var lifetime context.Context
	func() {
		defer func() {
			if recovered := recover(); recovered != failure {
				t.Errorf("panic = %v, want connector failure", recovered)
			}
		}()
		_, _, _ = connectSession(t.Context(), t.Context(), nil, func(ctx context.Context) (*sdkmcp.ClientSession, error) {
			lifetime = ctx
			panic(failure)
		})
	}()
	if lifetime == nil || lifetime.Err() != context.Canceled {
		t.Error("panicked handshake retained its untransferred session lifetime")
	}
}

func TestHandshakeRejectsLateSDKSessionAfterLifetimeCancellation(t *testing.T) {
	request, stopRequest := context.WithCancelCause(t.Context())
	defer stopRequest(nil)
	owner, stopOwner := context.WithCancelCause(t.Context())
	defer stopOwner(nil)
	for _, lifetime := range []struct {
		name          string
		parent, owner context.Context
		cancel        context.CancelCauseFunc
	}{
		{"handshake", request, t.Context(), stopRequest},
		{"session owner", t.Context(), owner, stopOwner},
	} {
		t.Run(lifetime.name, func(t *testing.T) {
			clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "handshake-test", Version: "v1"}, nil)
			serverSession, err := remote.Connect(t.Context(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := serverSession.Close(); err != nil {
					t.Error(err)
				}
			})
			cause := errors.New("handshake owner stopped")
			session, release, err := connectSession(lifetime.parent, lifetime.owner, nil, func(ctx context.Context) (*sdkmcp.ClientSession, error) {
				session, err := newClient().Connect(ctx, clientTransport, nil)
				if err == nil {
					t.Cleanup(func() {
						if err := session.Close(); err != nil {
							t.Error(err)
						}
					})
					lifetime.cancel(cause)
				}
				return session, err
			})
			if release != nil {
				t.Cleanup(release)
			}
			if session != nil || release != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("late session = (%v, %v, %v), want rejected lifetime and cause", session, release != nil, err)
			}
		})
	}
}

func TestProbeRetainsConfiguredHandshakeTimeout(t *testing.T) {
	transport := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			return
		}
		<-request.Context().Done()
	}))
	t.Cleanup(transport.Close)
	timeout, err := mcpserver.NewHandshakeTimeout(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	config := ServerConfig{Server: mcpserver.Server{
		Source: mcpserver.UserSource(), Name: testsupport.ServerName("probe"),
		Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL,
		HandshakeTimeout: timeout,
	}}
	if err := (*connections)(nil).probe(t.Context(), config); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe = %v, want configured handshake timeout", err)
	}
}
