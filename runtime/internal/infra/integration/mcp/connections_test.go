package mcp

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"

	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/scope/core/chat"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type catalogTool string

type oauthHandlerStub struct{}

type oauthLoadStore struct {
	memoryOAuthStore
	load func(context.Context, mcpserver.OAuthTarget) ([]byte, string, bool, error)
}

func (s *oauthLoadStore) LoadOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) ([]byte, string, bool, error) {
	return s.load(ctx, target)
}

func (oauthHandlerStub) TokenSource(context.Context) (oauth2.TokenSource, error) { return nil, nil }
func (oauthHandlerStub) Authorize(context.Context, *http.Request, *http.Response) error {
	return nil
}

func TestReusableOAuthIsBoundToCredentialConfiguration(t *testing.T) {
	handler := oauthHandlerStub{}
	current := ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("server"), Transport: TransportHTTP, Endpoint: "https://EXAMPLE.com/mcp",
	}
	if got := reusableOAuth(current, ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("server"), Transport: TransportHTTP, Endpoint: "https://EXAMPLE.com/mcp",
	}, handler); got == nil {
		t.Fatal("unchanged credential configuration did not preserve OAuth handler")
	}
	changedPath := current.Clone()
	changedPath.Endpoint = "https://EXAMPLE.com/other"
	if got := reusableOAuth(current, changedPath, handler); got != nil {
		t.Fatal("changed endpoint preserved an invalidated OAuth handler")
	}
	changedHeaders := current.Clone()
	changedHeaders.Headers = map[string]string{"X-API-Key": "changed"}
	if got := reusableOAuth(current, changedHeaders, handler); got != nil {
		t.Fatal("changed authentication headers preserved an invalidated OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("server"), Transport: TransportHTTP, Endpoint: "https://other.example/mcp",
	}, handler); got != nil {
		t.Fatal("cross-origin endpoint preserved OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("server"), Transport: TransportStdio, Command: "server",
	}, handler); got != nil {
		t.Fatal("transport change preserved OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("server"), Transport: TransportHTTP, Endpoint: "https://example.com/other",
		Authorization: "Bearer static",
	}, handler); got != nil {
		t.Fatal("static authorization preserved OAuth handler")
	}
}

func (c catalogTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: string(c), InputSchema: jsontext.Value(`{"type":"object"}`)}
}

func (catalogTool) Call(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
	return chat.ToolOutput{}, nil
}

func TestConnectionsRejectMutationsAfterShutdown(t *testing.T) {
	c := &Connections{lifetime: t.Context(), client: newClient()}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	cfg := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("closed"), Transport: TransportHTTP, Endpoint: "https://example.invalid"}
	for name, call := range map[string]func() error{
		"configure": func() error { return c.Configure(context.Background(), mustLaunch(t, cfg)) },
		"authorize": func() error { return c.Authorize(context.Background(), mustLaunch(t, cfg)) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrConnectionsClosed) {
				t.Fatalf("error = %v, want ErrConnectionsClosed", err)
			}
		})
	}

	if err := c.Detach(cfg.ID()); !errors.Is(err, ErrConnectionsClosed) {
		t.Fatalf("Detach after Shutdown = %v, want ErrConnectionsClosed", err)
	}
	if got := c.Statuses(); len(got) != 0 {
		t.Fatalf("statuses after Shutdown + Remove = %v, want empty", got)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestDialRequiresStartupAndProcessLifetimes(t *testing.T) {
	var missingContext context.Context
	if connections, _, err := testDial(missingContext, t.Context(), nil, nil); err == nil || connections != nil {
		t.Fatalf("Dial without startup context = (%v, %v), want nil connections and non-nil error", connections, err)
	}
	if connections, _, err := testDial(t.Context(), nil, nil, nil); err == nil || connections != nil {
		t.Fatalf("Dial without process lifetime = (%v, %v), want nil connections and non-nil error", connections, err)
	}
	if connections, err := Dial(t.Context(), t.Context(), nil, nil, nil); err == nil || connections != nil {
		t.Fatalf("Dial without configuration source = (%v, %v), want nil connections and non-nil error", connections, err)
	}
}

func TestConnectionsShutdownCancelsAndJoinsAttempts(t *testing.T) {
	c := &Connections{lifetime: t.Context()}
	target := &server{id: testsupport.UserMCPServer("server"), config: ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server")}}
	c.servers = []*server{target}
	c.mu.Lock()
	attempt := c.beginAttempt(t.Context(), target)
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := c.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown = %v, want context deadline exceeded", err)
	}
	select {
	case <-attempt.ctx.Done():
	default:
		t.Fatal("Shutdown did not cancel the active attempt")
	}

	c.finishAttempt(attempt)
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("join Shutdown: %v", err)
	}
}

func TestConnectionsShutdownSettlesTerminalSessionCloseError(t *testing.T) {
	closeErr := errors.New("session close failed")
	var calls atomic.Int32
	session := new(sdkmcp.ClientSession)
	owned := &ownedSession{
		// ClientSession.Close has this exact one-shot shape: its transport closer
		// is consumed even when it returns an error, so replay can only return the
		// same diagnostic and can never advance resource settlement.
		closeFn: sync.OnceValue(func() error {
			calls.Add(1)
			return closeErr
		}),
	}
	c := &Connections{
		lifetime: t.Context(),
		closed:   true,
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: owned},
	}

	if err := c.Shutdown(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("first Shutdown = %v, want close failure", err)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after terminal close error = %d, want 0", got)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown = %v, want settled no-op", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying session close calls = %d, want 1", got)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after repeated Shutdown = %d, want 0", got)
	}
}

func TestConnectionsShutdownReportsSettledAsyncRetirementDiagnosticOnce(t *testing.T) {
	closeErr := errors.New("retired session close failed")
	var calls atomic.Int32
	session := new(sdkmcp.ClientSession)
	c := &Connections{
		lifetime: t.Context(),
		servers: []*server{{
			id:     testsupport.UserMCPServer("retired"),
			config: ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("retired")}, session: session,
		}},
		sessions: map[*sdkmcp.ClientSession]*ownedSession{
			session: {
				closeFn: sync.OnceValue(func() error {
					calls.Add(1)
					return closeErr
				}),
			},
		},
	}

	if err := c.Detach(testsupport.UserMCPServer("retired")); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	c.mu.Lock()
	var closeAttempt *sessionCloseAttempt
	for candidate := range c.retirements {
		closeAttempt = candidate
		break
	}
	c.mu.Unlock()
	if closeAttempt == nil {
		t.Fatal("asynchronous retirement was not registered")
	}
	<-closeAttempt.done
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after terminal retirement error = %d, want 0", got)
	}

	if err := c.Shutdown(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("first Shutdown = %v, want retirement diagnostic", err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown = %v, want settled no-op", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying session close calls = %d, want 1", got)
	}
}

func TestConnectionAttemptsSupersedePerServer(t *testing.T) {
	c := &Connections{lifetime: t.Context()}
	first := &server{id: testsupport.UserMCPServer("first"), config: ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("first")}}
	second := &server{id: testsupport.UserMCPServer("second"), config: ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("second")}}
	c.servers = []*server{first, second}

	c.mu.Lock()
	oldFirst := c.beginAttempt(t.Context(), first)
	secondAttempt := c.beginAttempt(t.Context(), second)
	newFirst := c.beginAttempt(t.Context(), first)
	if !c.currentAttempt(newFirst) || c.currentAttempt(oldFirst) == true {
		t.Fatal("latest first-server attempt did not own its registration")
	}
	c.mu.Unlock()

	if oldFirst.ctx.Err() == nil {
		t.Fatal("new same-server attempt did not cancel its predecessor")
	}
	if secondAttempt.ctx.Err() != nil {
		t.Fatal("first-server attempt canceled an unrelated server")
	}
	c.finishAttempt(oldFirst)
	c.finishAttempt(secondAttempt)
	c.finishAttempt(newFirst)
}

func TestCloneServerConfigOwnsMutableFields(t *testing.T) {
	original := ServerConfig{
		Args:    []string{"one"},
		Env:     []string{"A=1"},
		Headers: map[string]string{"X-Test": "before"},
	}
	cloned := original.Clone()
	original.Args[0] = "two"
	original.Env[0] = "A=2"
	original.Headers["X-Test"] = "after"

	if cloned.Args[0] != "one" || cloned.Env[0] != "A=1" || cloned.Headers["X-Test"] != "before" {
		t.Fatalf("clone retained caller-owned storage: %+v", cloned)
	}
}

func TestToolSinkReceivesCurrentSnapshotOnRegistration(t *testing.T) {
	c := &Connections{lifetime: t.Context(), servers: []*server{
		{
			id:      testsupport.UserMCPServer("alpha"),
			config:  ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("alpha")},
			session: new(sdkmcp.ClientSession),
			tools:   []Executable{{Tool: catalogTool("alpha_read")}, {Tool: catalogTool("alpha_list")}},
		},
		{
			id:      testsupport.UserMCPServer("beta"),
			config:  ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("beta")},
			session: new(sdkmcp.ClientSession),
			tools:   []Executable{{Tool: catalogTool("beta_read")}},
		},
	}}
	var got []string
	c.SetToolSink(func(catalog []Executable) {
		got = make([]string, 0, len(catalog))
		for _, tool := range catalog {
			got = append(got, tool.Definition().Name)
		}
	})

	want := []string{"alpha_read", "alpha_list", "beta_read"}
	if !slices.Equal(got, want) {
		t.Fatalf("published tools = %v, want %v", got, want)
	}
	c.servers = nil
	c.SetToolSink(func(catalog []Executable) { got = toolNames(catalog) })
	if len(got) != 0 {
		t.Fatalf("replacement sink retained withdrawn tools: %v", got)
	}
}

func TestDetachPublishesRemainingSnapshot(t *testing.T) {
	c := &Connections{lifetime: t.Context(), servers: []*server{
		{id: testsupport.UserMCPServer("remove"), config: ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remove")}, tools: []Executable{{Tool: catalogTool("remove_read")}}},
		{
			id:      testsupport.UserMCPServer("keep"),
			config:  ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("keep")},
			session: new(sdkmcp.ClientSession),
			tools:   []Executable{{Tool: catalogTool("keep_read")}},
		},
	}}
	published := make(chan []string, 1)
	c.SetToolSink(func(catalog []Executable) {
		names := make([]string, 0, len(catalog))
		for _, tool := range catalog {
			names = append(names, tool.Definition().Name)
		}
		published <- names
	})
	if got := <-published; !slices.Equal(got, []string{"keep_read"}) {
		t.Fatalf("initial publication = %v, want [keep_read]", got)
	}
	if err := c.Detach(testsupport.UserMCPServer("remove")); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if got := <-published; !slices.Equal(got, []string{"keep_read"}) {
		t.Fatalf("published tools = %v, want [keep_read]", got)
	}
}

func TestReconnectPublishesRemovalBeforeVerifiedReplacement(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "first")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if len(initial) != 1 || initial[0].Definition().Name != "remote_first" {
		t.Fatalf("initial tools = %v, want [remote_first]", toolNames(initial))
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []Executable) { publications <- toolNames(catalog) })
	if got := <-publications; !slices.Equal(got, toolNames(initial)) {
		t.Fatalf("initial publication = %v, want %v", got, toolNames(initial))
	}
	addRemoteTool(t, remote, "second")
	if listed := liveToolNames(c, config.ID()); !slices.Equal(listed, []string{"remote_first"}) {
		t.Fatalf("catalog before reconnect = %v; want the admitted first tool", listed)
	}
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if connecting := <-publications; len(connecting) != 0 {
		t.Fatalf("connecting publication = %v, want empty", connecting)
	}
	settled := <-publications
	slices.Sort(settled)
	if want := []string{"remote_first", "remote_second"}; !slices.Equal(settled, want) {
		t.Fatalf("settled publication = %v, want %v", settled, want)
	}
	if listed := liveToolNames(c, config.ID()); len(listed) != len(settled) {
		t.Fatalf("catalog after reconnect = %v; want the replacement catalog", listed)
	}
}

func TestConfigureOAuthRestoreFailureWithdrawsPreviousConnection(t *testing.T) {
	loadErr := errors.New("credential storage unavailable")
	closeErr := errors.New("old session close failed")
	var closed atomic.Int32
	session := new(sdkmcp.ClientSession)
	config := ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: "https://example.invalid/mcp",
	}
	c := &Connections{
		lifetime: t.Context(), client: newClient(),
		oauthSessions: &oauthLoadStore{load: func(context.Context, mcpserver.OAuthTarget) ([]byte, string, bool, error) {
			return nil, "", false, loadErr
		}},
		servers: []*server{{
			id: config.ID(), config: config, session: session, tools: []Executable{{Tool: catalogTool("remote_read")}},
			state: mcpserver.ConnectionConnected,
		}},
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error {
			closed.Add(1)
			return closeErr
		}}},
	}
	var publications [][]string
	c.SetToolSink(func(catalog []Executable) { publications = append(publications, toolNames(catalog)) })
	config.Endpoint = "https://example.invalid/replacement"
	if err := c.Configure(t.Context(), mustLaunch(t, config)); !errors.Is(err, loadErr) || !errors.Is(err, closeErr) {
		t.Errorf("Configure = %v, want credential and retirement errors", err)
	}
	if statuses := c.Statuses(); len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionFailed || statuses[0].ToolCount != 0 || statuses[0].Failure != mcpserver.FailureConfiguration {
		t.Errorf("statuses = %+v, want one source failed in configuration without tools", statuses)
	}
	if len(publications) != 2 || !slices.Equal(publications[0], []string{"remote_read"}) || len(publications[1]) != 0 {
		t.Errorf("publications = %v, want the old tools withdrawn", publications)
	}
	if closed.Load() != 1 || ownedSessionCount(c) != 0 {
		t.Errorf("old session retirement = %d closes, %d owned sessions", closed.Load(), ownedSessionCount(c))
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestShutdownCancelsAndJoinsOAuthRestore(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishLoad := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishLoad)
	c := &Connections{
		lifetime: t.Context(), client: newClient(),
		oauthSessions: &oauthLoadStore{load: func(ctx context.Context, _ mcpserver.OAuthTarget) ([]byte, string, bool, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
			return nil, "", false, ctx.Err()
		}},
	}
	configureDone := make(chan error, 1)
	configureCtx, cancelConfigure := context.WithCancel(t.Context())
	defer cancelConfigure()
	go func() {
		configureDone <- c.Configure(configureCtx, mustLaunch(t, ServerConfig{
			Source: mcpserver.UserSource(),
			Name:   testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: "https://example.invalid/mcp",
		}))
	}()
	<-started
	shutdownCtx, cancelShutdown := context.WithCancel(t.Context())
	defer cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- c.Shutdown(shutdownCtx) }()
	select {
	case <-canceled:
		cancelShutdown()
		if err := <-shutdownDone; !errors.Is(err, context.Canceled) {
			t.Errorf("Shutdown during restoration = %v, want canceled wait", err)
		}
	case err := <-shutdownDone:
		t.Errorf("Shutdown returned %v before canceling and joining OAuth restoration", err)
		cancelConfigure()
		<-canceled
	}
	cancelConfigure()
	finishLoad()
	if err := c.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown after restoration: %v", err)
	}
	if err := <-configureDone; !errors.Is(err, context.Canceled) {
		t.Errorf("Configure = %v, want cancellation", err)
	}
}

func TestSupersededOAuthRestoreCannotReplaceCurrentConnection(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote }, nil,
	))
	t.Cleanup(httpServer.Close)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishLoad := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishLoad)
	c, _, err := testDial(t.Context(), t.Context(), nil, &oauthLoadStore{
		load: func(context.Context, mcpserver.OAuthTarget) ([]byte, string, bool, error) {
			close(started)
			<-release
			return nil, "", false, nil
		},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		finishLoad()
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	previousDone := make(chan error, 1)
	go func() { previousDone <- c.Configure(t.Context(), mustLaunch(t, config)) }()
	<-started
	config.Authorization = "Bearer replacement"
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatalf("replacement Configure: %v", err)
	}
	finishLoad()
	if err := <-previousDone; !errors.Is(err, errConnectionSuperseded) {
		t.Errorf("previous Configure = %v, want superseded operation", err)
	}
	if statuses := c.Statuses(); len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnected || statuses[0].ToolCount != 1 {
		t.Errorf("statuses = %+v, want the verified replacement connection", statuses)
	}
	if tools := liveToolNames(c, config.ID()); !slices.Equal(tools, []string{"remote_read"}) {
		t.Errorf("replacement tools = %v", tools)
	}
}

func TestCanceledSessionCloseRetainsFailureForShutdown(t *testing.T) {
	closeErr := errors.New("session close failed after cancellation")
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishClose := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishClose)
	session := new(sdkmcp.ClientSession)
	c := &Connections{
		lifetime: t.Context(), client: newClient(),
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error {
			<-release
			return closeErr
		}}},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.closeSession(ctx, session); !errors.Is(err, context.Canceled) {
		t.Fatalf("closeSession = %v, want canceled wait", err)
	}
	c.mu.Lock()
	closeAttempt := c.sessions[session].close
	c.mu.Unlock()
	finishClose()
	<-closeAttempt.done
	if err := c.Shutdown(t.Context()); !errors.Is(err, closeErr) {
		t.Errorf("Shutdown = %v, want unreported retirement error", err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Errorf("settled Shutdown = %v, want no-op", err)
	}
}

func TestDetachCancelsAuthorizationDuringSessionRetirement(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishClose := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishClose)
	session := new(sdkmcp.ClientSession)
	config := ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: "https://example.invalid/mcp",
	}
	c := &Connections{
		lifetime: t.Context(), client: newClient(),
		servers: []*server{{id: config.ID(), config: config, session: session, state: mcpserver.ConnectionConnected}},
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error {
			close(started)
			<-release
			return nil
		}}},
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	authorizationDone := make(chan error, 1)
	go func() { authorizationDone <- c.Authorize(ctx, mustLaunch(t, config)) }()
	<-started
	if err := c.Detach(config.ID()); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	guard, cancelGuard := context.WithTimeout(t.Context(), time.Second)
	defer cancelGuard()
	select {
	case err := <-authorizationDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Authorize = %v, want canceled attempt", err)
		}
	case <-guard.Done():
		t.Error("detached authorization still waits for session retirement")
		cancel()
		<-authorizationDone
	}
	finishClose()
	if err := c.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestConfiguredSessionOutlivesRequestScope(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	connections, _, err := testDial(t.Context(), t.Context(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if shutdownErr := connections.Shutdown(context.WithoutCancel(t.Context())); shutdownErr != nil {
			t.Errorf("Shutdown: %v", shutdownErr)
		}
	})

	requestCtx, cancelRequest := context.WithCancel(t.Context())
	if configureErr := connections.Configure(requestCtx, mustLaunch(t, ServerConfig{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("dynamic"), Transport: TransportHTTP, Endpoint: httpServer.URL,
	})); configureErr != nil {
		t.Fatalf("Configure: %v", configureErr)
	}
	cancelRequest()

	if tools := liveToolNames(connections, testsupport.UserMCPServer("dynamic")); !slices.Equal(tools, []string{"dynamic_read"}) {
		t.Fatalf("Tools after request scope ended = %v, want dynamic_read", tools)
	}
}

func TestSessionLedgerOwnsReplacementUntilClose(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("ledger"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, _, err := testDial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 1 {
		t.Fatalf("owned sessions after Dial = %d, want 1", got)
	}
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 1 {
		t.Fatalf("owned sessions after replacement = %d, want 1", got)
	}
	if err := c.Detach(config.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after Detach + Shutdown = %d, want 0", got)
	}
}

func ownedSessionCount(c *Connections) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sessions)
}

func TestDialAdmitsCrossServerPublicToolNameCollision(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a.b"), Transport: TransportHTTP, Endpoint: httpServer.URL},
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a_b"), Transport: TransportHTTP, Endpoint: httpServer.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_read", "a_b_read"}) {
		t.Fatalf("initial tools = %v, want both servers' tools", names)
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionConnected {
		t.Fatalf("statuses = %+v, want both connected", statuses)
	}
}

func TestDialReportsStartupFailureAndKeepsHealthyServers(t *testing.T) {
	for _, failure := range []string{"admission", "connection"} {
		t.Run(failure, func(t *testing.T) {
			var diagnostics bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })

			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "healthy", Version: "v1"}, nil)
			addRemoteTool(t, remote, "read")
			httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
				func(*http.Request) *sdkmcp.Server { return remote }, nil,
			))
			t.Cleanup(httpServer.Close)
			missingCommand := filepath.Join(t.TempDir(), "missing-mcp-server")
			configs := []ServerConfig{
				{Source: mcpserver.UserSource(), Name: testsupport.ServerName("missing"), Transport: TransportStdio, Command: missingCommand},
				{Source: mcpserver.UserSource(), Name: testsupport.ServerName("healthy"), Transport: TransportHTTP, Endpoint: httpServer.URL},
			}
			connections, err := Dial(t.Context(), t.Context(), configs, nil, func(_ context.Context, name mcpserver.ID) (*Launch, error) {
				if name == configs[0].ID() && failure == "admission" {
					return nil, errors.New("release rejected before connection")
				}
				for _, config := range configs {
					if config.ID() == name {
						return NewLaunch(config, nil, nil)
					}
				}
				return nil, mcpserver.ErrUnknownServer
			})
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			t.Cleanup(func() {
				if err := connections.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Errorf("Shutdown: %v", err)
				}
			})
			var initial []Executable
			connections.SetToolSink(func(catalog []Executable) { initial = catalog })
			if names := toolNames(initial); !slices.Equal(names, []string{"healthy_read"}) {
				t.Fatalf("initial tools = %v, want healthy server's tool", names)
			}
			statuses := connections.Statuses()
			if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionFailed ||
				statuses[1].State != mcpserver.ConnectionConnected {
				t.Fatalf("statuses = %+v, want failed then connected", statuses)
			}
			wantFailure := mcpserver.FailureConnection
			if failure == "admission" {
				wantFailure = mcpserver.FailureConfiguration
			}
			if statuses[0].Failure != wantFailure || statuses[1].Failure != "" {
				t.Fatalf("failure categories = %q, %q; want %q for the failed server only", statuses[0].Failure, statuses[1].Failure, wantFailure)
			}
			cause := missingCommand
			if failure == "admission" {
				cause = "release rejected before connection"
			}
			if output := diagnostics.String(); !strings.Contains(output, "server.name=missing") || !strings.Contains(output, cause) {
				t.Fatalf("startup failure lost its server or cause: %s", output)
			}
			if tools := liveToolNames(connections, testsupport.UserMCPServer("healthy")); !slices.Equal(tools, []string{"healthy_read"}) {
				t.Fatalf("healthy server tools after startup failure = %v", tools)
			}
		})
	}
}

func TestConfigureAdmitsCrossServerPublicToolNameCollision(t *testing.T) {
	firstRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "first", Version: "v1"}, nil)
	addRemoteTool(t, firstRemote, "c")
	firstHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return firstRemote },
		nil,
	))
	t.Cleanup(firstHTTP.Close)

	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{{
		Source: mcpserver.UserSource(),
		Name:   testsupport.ServerName("a_b"), Transport: TransportHTTP, Endpoint: firstHTTP.URL,
	}}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() {
		if shutdownErr := c.Shutdown(context.WithoutCancel(t.Context())); shutdownErr != nil {
			t.Errorf("Shutdown: %v", shutdownErr)
		}
	}()
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_c"}) {
		t.Fatalf("initial tools = %v, want [a_b_c]", names)
	}

	secondRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "second", Version: "v1"}, nil)
	addRemoteTool(t, secondRemote, "b_c")
	secondHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return secondRemote },
		nil,
	))
	t.Cleanup(secondHTTP.Close)

	err = c.Configure(t.Context(), mustLaunch(t, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a"), Transport: TransportHTTP, Endpoint: secondHTTP.URL}))
	if err != nil {
		t.Fatalf("Configure collision error = %v", err)
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionConnected {
		t.Fatalf("statuses = %+v, want both connected", statuses)
	}
}

func TestReconnectAdmitsNewCrossServerPublicToolNameCollision(t *testing.T) {
	firstRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "first", Version: "v1"}, nil)
	addRemoteTool(t, firstRemote, "c")
	firstHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return firstRemote },
		nil,
	))
	t.Cleanup(firstHTTP.Close)

	secondRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "second", Version: "v1"}, nil)
	addRemoteTool(t, secondRemote, "safe")
	secondHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return secondRemote },
		nil,
	))
	t.Cleanup(secondHTTP.Close)

	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a_b"), Transport: TransportHTTP, Endpoint: firstHTTP.URL},
		{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a"), Transport: TransportHTTP, Endpoint: secondHTTP.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if shutdownErr := c.Shutdown(context.WithoutCancel(t.Context())); shutdownErr != nil {
			t.Errorf("Shutdown: %v", shutdownErr)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_c", "a_safe"}) {
		t.Fatalf("initial tools = %v, want [a_b_c a_safe]", names)
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []Executable) { publications <- toolNames(catalog) })
	if got := <-publications; !slices.Equal(got, toolNames(initial)) {
		t.Fatalf("initial publication = %v, want %v", got, toolNames(initial))
	}
	addRemoteTool(t, secondRemote, "b_c")
	err = c.Configure(t.Context(), mustLaunch(t, ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("a"), Transport: TransportHTTP, Endpoint: secondHTTP.URL}))
	if err != nil {
		t.Fatalf("Reconnect collision error = %v", err)
	}
	for phase, want := range [][]string{{"a_b_c"}, {"a_b_c", "a_b_c", "a_safe"}} {
		if names := <-publications; !slices.Equal(names, want) {
			t.Fatalf("publication %d = %v, want %v", phase, names, want)
		}
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionConnected {
		t.Fatalf("statuses = %+v, want both connected", statuses)
	}
}

func addRemoteTool(t *testing.T, server *sdkmcp.Server, name string) {
	t.Helper()
	tool, err := toolcontract.NewFunc[struct{}, string](toolcontract.FuncConfig{Name: name}, func(context.Context, struct{}) (string, error) {
		return name, nil
	})
	if err != nil {
		t.Fatalf("build remote tool %q: %v", name, err)
	}
	if err := scopemcp.Register(server, tool); err != nil {
		t.Fatalf("register remote tool %q: %v", name, err)
	}
}

// liveToolNames reads the tools a publication would carry for one server now.
func liveToolNames(c *Connections, name mcpserver.ID) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	if configuredServer := c.find(name); configuredServer != nil && configuredServer.session != nil {
		names = toolNames(configuredServer.tools)
	}
	return names
}

func toolNames(catalog []Executable) []string {
	names := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		names = append(names, tool.Definition().Name)
	}
	return names
}

func TestCanceledConnectionCommandsPreserveTheCurrentSession(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "first")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("detach"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	var publications [][]string
	c.SetToolSink(func(catalog []Executable) { publications = append(publications, toolNames(catalog)) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, test := range []struct {
		name    string
		command func() error
	}{
		{"configure", func() error { return c.Configure(ctx, mustLaunch(t, config)) }},
		{"refuse", func() error { return c.Refuse(ctx, config.ID(), mcpserver.FailureConfiguration) }},
		{"authorize", func() error { return c.Authorize(ctx, mustLaunch(t, config)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			name, command := test.name, test.command
			if err := command(); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled %s = %v, want cancellation", name, err)
			}
			if len(publications) != 1 || !slices.Equal(publications[0], toolNames(initial)) {
				t.Fatalf("canceled %s published tools: %v", name, publications)
			}
			if tools := liveToolNames(c, config.ID()); !slices.Equal(tools, []string{"detach_first"}) {
				t.Fatalf("tools after canceled %s = %v", name, tools)
			}
			if len(initial) != 1 || !initial[0].Current() {
				t.Fatalf("canceled %s retired the current executable", name)
			}
			if statuses := c.Statuses(); len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnected || statuses[0].ToolCount != 1 {
				t.Fatalf("status after canceled %s = %+v", name, statuses)
			}
		})
	}
}

func testDial(ctx, lifetime context.Context, servers []ServerConfig, oauthSessions OAuthSessionStore) (*Connections, []Executable, error) {
	c, err := Dial(ctx, lifetime, servers, oauthSessions, func(_ context.Context, name mcpserver.ID) (*Launch, error) {
		for _, config := range servers {
			if config.ID() == name {
				return NewLaunch(config, nil, nil)
			}
		}
		return nil, mcpserver.ErrUnknownServer
	})
	if err != nil {
		return nil, nil, err
	}
	var initial []Executable
	c.SetToolSink(func(catalog []Executable) { initial = catalog })
	c.SetToolSink(nil)
	return c, initial, nil
}

// The sink and Statuses describe one fact. Publication inside the critical
// section that changes a live outcome means no reader can observe one without
// the other; the lock must therefore be held whenever the sink runs.
func TestToolPublicationHappensInsideTheSettlingCriticalSection(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(httpServer.Close)
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, _, err := testDial(t.Context(), t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var publications [][]string
	c.SetToolSink(func(catalog []Executable) {
		if c.mu.TryLock() {
			c.mu.Unlock()
			t.Error("tool sink ran outside the critical section that settled the connection")
		}
		publications = append(publications, toolNames(catalog))
	})
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	if err := c.Refuse(t.Context(), config.ID(), mcpserver.FailureConfiguration); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	if err := c.Detach(config.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{}, {}, {"remote_read"}, {}, {}, {"remote_read"}, {}, {}, {"remote_read"}, {}}
	if !slices.EqualFunc(publications, want, slices.Equal) {
		t.Fatalf("publications = %v, want %v", publications, want)
	}
}

// A configuration read before its source was detached belongs to a
// superseded operation. Its owner canceled it before detaching, so it must
// not re-add the server as a failed or connecting ghost.
func TestSupersededConfigureNeverResurrectsADetachedServer(t *testing.T) {
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: "https://example.invalid/mcp"}
	c := &Connections{lifetime: t.Context(), client: newClient(), servers: []*server{{id: config.ID(), config: config, state: mcpserver.ConnectionFailed, failure: mcpserver.FailureConnection}}}
	published := false
	c.SetToolSink(func([]Executable) { published = true })
	ctx, supersede := context.WithCancel(t.Context())
	supersede()
	if err := c.Detach(config.ID()); err != nil {
		t.Fatal(err)
	}
	published = false
	if err := c.Configure(ctx, mustLaunch(t, config)); !errors.Is(err, context.Canceled) {
		t.Fatalf("superseded Configure = %v, want cancellation", err)
	}
	if statuses := c.Statuses(); len(statuses) != 0 || published {
		t.Fatalf("detached server resurrected: %+v, published=%t", statuses, published)
	}
}

func TestExecutableIsCurrentOnlyWhileItsSessionServesTheSource(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(httpServer.Close)
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, initial, err := testDial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Shutdown(context.WithoutCancel(t.Context())) })
	if len(initial) != 1 || !initial[0].Current() {
		t.Fatalf("admitted executable is not current: %v", toolNames(initial))
	}
	var replacement []Executable
	c.SetToolSink(func(catalog []Executable) { replacement = catalog })
	if err := c.Refuse(t.Context(), config.ID(), mcpserver.FailureConfiguration); err != nil {
		t.Fatal(err)
	}
	if initial[0].Current() {
		t.Fatal("refusal left the withdrawn session current")
	}
	if err := c.Configure(t.Context(), mustLaunch(t, config)); err != nil {
		t.Fatal(err)
	}
	if len(replacement) != 1 || !replacement[0].Current() {
		t.Fatalf("reconnected executable is not current: %v", toolNames(replacement))
	}
	if initial[0].Current() {
		t.Fatal("reconnecting with the same configuration revived the closed session")
	}
	if probe := (Executable{config: config}); probe.Current() {
		t.Fatal("an executable without a live owner is current")
	}
}

func mustLaunch(t testing.TB, config ServerConfig) *Launch {
	t.Helper()
	launch, err := NewLaunch(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return launch
}
