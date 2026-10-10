package mcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestAdmissionWithdrawsToolsBeforeConfigurationIO(t *testing.T) {
	name := testsupport.UserMCPServer("server")
	session := new(sdkmcp.ClientSession)
	var retired atomic.Int32
	c := &Connections{
		lifetime: t.Context(),
		servers:  []*server{{id: name, session: session, state: mcpserver.ConnectionConnected, tools: []Executable{{Tool: catalogTool("read")}}}},
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error { retired.Add(1); return nil }}},
	}
	var tools []string
	c.SetToolSink(func(catalog []Executable) { tools = toolNames(catalog) })
	attempt, err := c.BeginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*Launch, error) {
		t.Error("an unstarted attempt resolved configuration")
		return nil, errors.New("unexpected resolution")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("admission still exposes tools: %v", tools)
	}
	want := []mcpserver.ConnectionStatus{{Server: name, State: mcpserver.ConnectionConnecting}}
	if statuses := c.Statuses(); !slices.Equal(statuses, want) {
		t.Fatalf("admission status = %+v, want %+v", statuses, want)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); !errors.Is(err, context.Canceled) || retired.Load() != 1 {
		t.Fatalf("unstarted retirement = %v, closes = %d", err, retired.Load())
	}
}

func TestStaleConfigurationRefusalCannotWithdrawSuccessor(t *testing.T) {
	name := testsupport.UserMCPServer("server")
	c := &Connections{lifetime: t.Context()}
	entered, release := make(chan struct{}), make(chan struct{})
	previous, err := c.BeginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*Launch, error) {
		close(entered)
		<-release
		return nil, ErrSourceWithdrawn
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- previous.Run() }()
	<-entered
	successor, err := c.BeginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*Launch, error) {
		t.Error("successor resolved before Run")
		return nil, errors.New("unexpected resolution")
	})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrSourceWithdrawn) || !errors.Is(err, context.Canceled) {
		t.Fatalf("stale refusal = %v", err)
	}
	want := []mcpserver.ConnectionStatus{{Server: name, State: mcpserver.ConnectionConnecting}}
	if statuses := c.Statuses(); !slices.Equal(statuses, want) {
		t.Fatalf("stale refusal changed successor: %+v", statuses)
	}
	successor.Close()
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptPanicSettlesAndUnblocksShutdown(t *testing.T) {
	name := testsupport.UserMCPServer("server")
	c := &Connections{lifetime: t.Context()}
	attempt, err := c.BeginConnect(t.Context(), name, func(context.Context, mcpserver.ID) (*Launch, error) {
		panic("configuration source failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Run(); err == nil || !strings.Contains(err.Error(), "configuration source failed") {
		t.Fatalf("panic result = %v", err)
	}
	want := []mcpserver.ConnectionStatus{{Server: name, State: mcpserver.ConnectionFailed, Failure: mcpserver.FailureConfiguration}}
	if statuses := c.Statuses(); !slices.Equal(statuses, want) {
		t.Fatalf("panic left an unsettled attempt: %+v", statuses)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
