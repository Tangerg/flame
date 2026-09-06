package mcp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestReconnectFailureRemainsVisibleAfterRequestSpanEnds(t *testing.T) {
	var diagnostics bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	requestCtx, span := provider.Tracer("mcp-test").Start(t.Context(), "reconnect-request")
	defer span.End()
	name := testMCPServerName("files")
	dialErr := errors.New("endpoint unavailable after request returned")
	ports := &delayedFailingConnection{
		fakePorts: fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionFailed}}},
		release:   make(chan struct{}), err: dialErr,
	}
	states := make(chan mcpserver.ConnectionState, 2)
	var coordinator *Coordinator
	cfg := configWithPorts(ports)
	cfg.Invalidations = func(invalidation.Notice) {
		states <- testServerStatus(coordinator, name).State
	}
	coordinator = testCoordinator(t, cfg)
	if err := coordinator.ReconnectServer(requestCtx, name); err != nil {
		t.Fatal(err)
	}
	span.End()
	close(ports.release)
	for _, want := range []mcpserver.ConnectionState{mcpserver.ConnectionConnecting, mcpserver.ConnectionFailed} {
		select {
		case got := <-states:
			if got != want {
				t.Fatalf("connection state = %s, want %s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("connection state did not settle")
		}
	}
	requireCoordinatorShutdown(t, coordinator)
	if output := diagnostics.String(); !strings.Contains(output, dialErr.Error()) || !strings.Contains(output, name.String()) {
		t.Fatalf("background connection failure lost its server or cause: %s", output)
	}
}

type delayedFailingConnection struct {
	fakePorts
	release chan struct{}
	err     error
}

func (p *delayedFailingConnection) Reconnect(ctx context.Context, _ mcpserver.ServerName) error {
	select {
	case <-p.release:
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
