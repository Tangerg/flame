package terminal

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

const terminalMCPAuthorizationAttemptID = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"

type mcpServiceStub struct {
	mu          sync.Mutex
	servers     []protocol.MCPServer
	created     chan mcp.Candidate
	probed      chan mcp.Candidate
	updated     chan mcp.ServerUpdate
	deleted     chan string
	reconnected chan string
	authReads   atomic.Int32
	authErrors  chan error
	now         time.Time
}

type blockingMCPAuthorizationService struct {
	*mcpServiceStub
	started  chan struct{}
	release  chan struct{}
	canceled chan struct{}
}

type blockingMCPReconnectService struct {
	*mcpServiceStub
	started  chan string
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingMCPReconnectService) ReconnectServer(ctx context.Context, server string) error {
	select {
	case b.started <- server:
	default:
	}
	select {
	case <-b.release:
		return b.mcpServiceStub.ReconnectServer(ctx, server)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return context.Cause(ctx)
	}
}

func (b *blockingMCPAuthorizationService) GetAuthorization(
	ctx context.Context,
	reference mcp.AuthorizationReference,
) (protocol.MCPAuthorizationAttempt, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return b.mcpServiceStub.GetAuthorization(ctx, reference)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return protocol.MCPAuthorizationAttempt{}, context.Cause(ctx)
	}
}

func newMCPServiceStub() *mcpServiceStub {
	count := 1
	timeout := protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: new(15)}
	return &mcpServiceStub{
		servers: []protocol.MCPServer{{
			Name: "docs", Description: "Documentation", HandshakeTimeout: timeout,
			Connection: protocol.MCPConnection{Type: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools", AuthorizationMasked: "Bearer ****"},
			Status:     protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count},
		}},
		created: make(chan mcp.Candidate, 1), probed: make(chan mcp.Candidate, 1),
		updated: make(chan mcp.ServerUpdate, 1),
		deleted: make(chan string, 1), reconnected: make(chan string, 1), now: time.Unix(100, 0),
	}
}

func (m *mcpServiceStub) Servers(context.Context) ([]protocol.MCPServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	servers := make([]protocol.MCPServer, len(m.servers))
	for index := range m.servers {
		servers[index] = cloneMCPServer(m.servers[index])
	}
	return servers, nil
}

func (m *mcpServiceStub) CreateServer(_ context.Context, candidate mcp.Candidate) (protocol.MCPServer, error) {
	if err := candidate.Validate(); err != nil {
		return protocol.MCPServer{}, err
	}
	m.created <- candidate.Clone()
	server := protocol.MCPServer{
		Name: candidate.Name, Description: candidate.Description, HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
		Connection:    protocol.MCPConnection{Type: candidate.Connection.Transport, URL: candidate.Connection.URL, Command: candidate.Connection.Command, Args: candidate.Connection.Args, Dir: candidate.Connection.Directory},
		DisabledTools: candidate.DisabledTools, AutoApproveTools: candidate.AutoApproveTools,
		Status: protocol.MCPServerState{Type: protocol.MCPServerDisconnected},
	}
	if seconds, bounded := candidate.HandshakeTimeout.Seconds(); bounded {
		server.HandshakeTimeout = protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: &seconds}
	}
	m.mu.Lock()
	m.servers = append(m.servers, server)
	m.mu.Unlock()
	return cloneMCPServer(server), nil
}

func (m *mcpServiceStub) UpdateServer(_ context.Context, update mcp.ServerUpdate) (protocol.MCPServer, error) {
	if err := update.Validate(); err != nil {
		return protocol.MCPServer{}, err
	}
	m.updated <- update
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.servers {
		server := &m.servers[index]
		if server.Name != update.Server {
			continue
		}
		if update.Enabled != nil {
			if *update.Enabled {
				server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
			} else {
				server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
			}
		}
		return cloneMCPServer(*server), nil
	}
	return protocol.MCPServer{}, errors.New("server not found")
}

func (m *mcpServiceStub) DeleteServer(_ context.Context, name string) error {
	m.deleted <- name
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.servers {
		if m.servers[index].Name == name {
			m.servers = append(m.servers[:index], m.servers[index+1:]...)
			return nil
		}
	}
	return errors.New("server not found")
}

func (m *mcpServiceStub) TestServer(_ context.Context, candidate mcp.Candidate) (protocol.MCPTestResult, error) {
	if err := candidate.Validate(); err != nil {
		return protocol.MCPTestResult{}, err
	}
	m.probed <- candidate.Clone()
	return protocol.MCPTestResult{OK: true}, nil
}

func (*mcpServiceStub) Tools(_ context.Context, server string) ([]protocol.MCPTool, error) {
	if server != "" && server != "docs" {
		return nil, errors.New("server not found")
	}
	return []protocol.MCPTool{{Server: "docs", Name: "search", Description: "Search docs", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (m *mcpServiceStub) ReconnectServer(_ context.Context, server string) error {
	m.reconnected <- server
	return nil
}

func (m *mcpServiceStub) StartAuthorization(_ context.Context, server string) (protocol.MCPAuthorizationAttempt, error) {
	return protocol.MCPAuthorizationAttempt{
		ID: terminalMCPAuthorizationAttemptID, Server: server,
		Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending}, CreatedAt: m.now,
	}, nil
}

func (m *mcpServiceStub) GetAuthorization(context.Context, mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error) {
	m.authReads.Add(1)
	select {
	case err := <-m.authErrors:
		return protocol.MCPAuthorizationAttempt{}, err
	default:
	}
	finished := m.now.Add(time.Second)
	return protocol.MCPAuthorizationAttempt{
		ID: terminalMCPAuthorizationAttemptID, Server: "docs",
		Status:    protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: m.now, FinishedAt: &finished,
	}, nil
}

func TestMCPAuthorizationObserverRecoversTransientReadsAndStopsOnAuthoritativeAbsence(t *testing.T) {
	service := newMCPServiceStub()
	service.authErrors = make(chan error, 2)
	service.authErrors <- fmt.Errorf("temporary authorization read failure: %w", agent.ErrDisconnected)
	service.authErrors <- fmt.Errorf("another temporary authorization read failure: %w", agent.ErrDisconnected)
	observer := mcpAuthorizationObserver{
		service: service, pollInterval: time.Nanosecond,
		recovery: testBackoff(t, time.Nanosecond, time.Nanosecond),
	}
	initial, err := service.StartAuthorization(t.Context(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	observed, err := observer.observe(t.Context(), initial)
	if err != nil || observed.Status.Type != protocol.MCPAuthorizationAttemptSucceeded || service.authReads.Load() != 3 {
		t.Fatalf("observe after transient failures = (%+v, %v), reads %d", observed, err, service.authReads.Load())
	}

	service.authReads.Store(0)
	service.authErrors <- protocol.ErrMCPAuthorizationAttemptNotFound
	if _, err := observer.observe(t.Context(), initial); !errors.Is(err, protocol.ErrMCPAuthorizationAttemptNotFound) {
		t.Fatalf("observe missing attempt = %v, want ErrMCPAuthorizationAttemptNotFound", err)
	}
	if reads := service.authReads.Load(); reads != 1 {
		t.Fatalf("missing attempt reads = %d, want no retry", reads)
	}

	service.authReads.Store(0)
	permanent := errors.New("authorization rejected")
	service.authErrors <- permanent
	if _, err := observer.observe(t.Context(), initial); !errors.Is(err, permanent) {
		t.Fatalf("observe permanent failure = %v, want original error", err)
	}
	if reads := service.authReads.Load(); reads != 1 {
		t.Fatalf("permanent failure reads = %d, want no retry", reads)
	}
}

func TestMCPAuthorizationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	baseService := newMCPServiceStub()
	service := &blockingMCPAuthorizationService{
		mcpServiceStub: baseService,
		started:        make(chan struct{}, 1),
		release:        make(chan struct{}),
		canceled:       make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)

	backend := runtimefixture.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, MCP: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/mcp-auth docs")
	host.Press(input.Enter)
	host.Shows(t, "status   pending")
	awaitValue(t, service.started, "MCP authorization observation")

	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: protocol.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "Authorization refresh installed"
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: protocol.RuntimeSessionsChanged, Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Press(input.Esc)
	host.Shows(t, title)
	select {
	case <-service.canceled:
		t.Fatal("same-session projection replacement canceled the application-owned MCP authorization")
	default:
	}

	release()
	host.Shows(t, "MCP authorization succeeded · docs")
	if service.authReads.Load() == 0 {
		t.Fatal("MCP authorization did not resume after the session projection replacement")
	}
	stop()
}

func TestMCPLifecycleMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	baseService := newMCPServiceStub()
	service := &blockingMCPReconnectService{
		mcpServiceStub: baseService,
		started:        make(chan string, 1),
		release:        make(chan struct{}),
		canceled:       make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)

	backend := runtimefixture.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, MCP: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/mcp-reconnect docs")
	host.Press(input.Enter)
	if server := awaitValue(t, service.started, "MCP reconnect mutation"); server != "docs" {
		t.Fatalf("reconnect server = %q, want docs", server)
	}

	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: protocol.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "MCP lifecycle refresh installed"
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: protocol.RuntimeSessionsChanged, Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Shows(t, title)
	select {
	case <-service.canceled:
		t.Fatal("same-session projection replacement canceled the application-owned MCP mutation")
	default:
	}

	release()
	host.Shows(t, "requesting MCP reconnect docs accepted")
	if server := awaitValue(t, service.reconnected, "completed MCP reconnect mutation"); server != "docs" {
		t.Fatalf("reconnected server = %q, want docs", server)
	}
	stop()
}

func TestMCPToolsDocumentFormatsRuntimeSchema(t *testing.T) {
	for _, test := range []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "empty object", schema: map[string]any{}, want: "{}"},
		{name: "object", schema: map[string]any{"type": "object"}, want: "{\n  \"type\": \"object\"\n}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := mcpToolsDocument("docs", []protocol.MCPTool{{Server: "docs", Name: "search", InputSchema: test.schema}})
			if err != nil {
				t.Fatal(err)
			}
			if len(document.Sections) != 2 || document.Sections[1].Title != "Input schema" || document.Sections[1].Text != test.want {
				t.Fatalf("schema document = %+v, want %q", document, test.want)
			}
		})
	}
	malformed := protocol.MCPTool{Server: "docs", Name: "search", InputSchema: map[string]any{"invalid": func() {}}}
	if document, err := mcpToolsDocument("docs", []protocol.MCPTool{malformed}); err == nil || len(document.Sections) != 0 {
		t.Fatalf("malformed schema document = (%+v, %v), want no partial document", document, err)
	}
}

func TestMCPReadersFormsAndLifecycleCommands(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), MCP: service})
	host.Shows(t, "Ask flame")
	host.Type("/mcp")
	host.Press(input.Enter)
	host.Shows(t, "docs · connected · 1 tools")
	host.Shows(t, "Bearer ****")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/mcp-tools docs")
	host.Press(input.Enter)
	host.Shows(t, "docs/search")
	host.Shows(t, "Input schema")
	host.Shows(t, "object")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")

	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server")
	host.Type("private-docs")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 2/3")
	host.Shows(t, "HTTP URL")
	host.Type("https://private.example/tools")
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := "MCP_SECRET_42"
	host.Type(secret)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("MCP form did not survive a minimal viewport")
	}
	host.Shows(t, "Create MCP server")
	if strings.Contains(host.Frames(), secret) {
		t.Fatal("MCP authorization secret appeared in terminal frames")
	}
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 3/3")
	host.Shows(t, "Disabled tools")
	host.Press(input.Enter)
	host.Shows(t, "private-docs · disconnected")
	created := <-service.created
	if created.Connection.Authorization == nil || created.Connection.Authorization.Value != secret {
		t.Fatalf("created MCP candidate = %+v", created)
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")

	host.Type("/mcp-edit docs")
	host.Press(input.Enter)
	host.Shows(t, "Configure MCP server · docs")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "Configure MCP server · docs · 2/2")
	host.Press(input.Enter)
	host.Shows(t, "docs · disabled")
	updated := <-service.updated
	if updated.Enabled == nil || *updated.Enabled {
		t.Fatalf("MCP update = %+v", updated)
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/mcp-reconnect docs")
	host.Press(input.Enter)
	host.Shows(t, "requesting MCP reconnect docs accepted")
	if reconnected := <-service.reconnected; reconnected != "docs" {
		t.Fatalf("reconnected = %q", reconnected)
	}

	host.Type("/mcp-auth docs")
	host.Press(input.Enter)
	host.Shows(t, "complete the sign-in in your browser")
	host.Shows(t, "status   succeeded")
	if service.authReads.Load() == 0 {
		t.Fatal("MCP authorization was not polled")
	}
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/mcp-delete docs")
	host.Press(input.Enter)
	host.Shows(t, "Delete MCP server")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "deleting MCP server docs accepted")
	if deleted := <-service.deleted; deleted != "docs" {
		t.Fatalf("deleted = %q", deleted)
	}
	stop()
}

func TestMCPProbeValidatesAnUnpersistedCandidateAcrossResize(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), MCP: service})
	host.Shows(t, "Ask flame")
	host.Type("/mcp-probe")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 1/3")
	host.Type("probe-docs")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 2/3")
	host.Type("https://probe.example/tools")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("MCP probe form did not survive a minimal viewport")
	}
	host.Shows(t, "Test MCP candidate · 2/3")
	host.Press(input.Enter)
	host.Shows(t, "Test MCP candidate · 3/3")
	host.Press(input.Enter)
	host.Shows(t, "MCP candidate is reachable · probe-docs")
	probed := awaitValue(t, service.probed, "MCP candidate probe")
	if probed.Name != "probe-docs" || probed.Connection.Transport != protocol.MCPTransportStreamableHTTP ||
		probed.Connection.URL != "https://probe.example/tools" {
		t.Fatalf("probed MCP candidate = %+v", probed)
	}
	servers, err := service.Servers(t.Context())
	if err != nil || len(servers) != 1 || servers[0].Name != "docs" {
		t.Fatalf("MCP probe persisted candidate: servers=(%+v, %v)", servers, err)
	}
	stop()
}

func TestMCPStdioWizardKeepsEveryFieldVisibleAndSecretsMasked(t *testing.T) {
	service := newMCPServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), MCP: service})
	host.Shows(t, "Ask flame")
	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 1/3")
	host.Type("local-tools")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	showsPlain(t, host, "● stdio process")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 2/3")
	host.Shows(t, "stdio command")
	host.Type("local-mcp")
	host.Press(input.Tab)
	host.Type(`["--stdio"]`)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := `{"TOKEN":"MCP_STDIO_SECRET"}`
	host.Type(secret)
	host.Press(input.Tab)
	host.Type("/tmp")
	if !host.Resize(44, 18) || !host.Resize(96, 28) {
		t.Fatal("stdio MCP step did not survive a narrow viewport round trip")
	}
	if strings.Contains(host.Frames(), "MCP_STDIO_SECRET") {
		t.Fatal("stdio environment secret appeared in terminal frames")
	}
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 3/3")
	host.Press(input.Tab)
	host.Type("read, read")
	host.Press(input.Enter)
	host.Shows(t, `tool "read" is duplicated`)
	select {
	case candidate := <-service.created:
		t.Fatalf("invalid MCP policy reached the service: %+v", candidate)
	default:
	}
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("read")
	host.Press(input.Enter)
	host.Shows(t, "local-tools · disconnected")
	created := <-service.created
	if created.Connection.Transport != protocol.MCPTransportStdio || created.Connection.Command != "local-mcp" ||
		!slices.Equal(created.Connection.Args, []string{"--stdio"}) || created.Connection.Directory != "/tmp" ||
		created.Connection.Environment == nil || created.Connection.Environment.Value["TOKEN"] != "MCP_STDIO_SECRET" ||
		!slices.Equal(created.DisabledTools, []string{"read"}) {
		t.Fatalf("created stdio MCP candidate = %+v", created)
	}

	host.Press(input.Esc)
	stop()
}

func TestMCPChangedRefetchesTheOpenServerReader(t *testing.T) {
	service := newMCPServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []protocol.RuntimeTopic{protocol.TopicMCPChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), MCP: service, Changes: source})
	host.Shows(t, "Ask flame")
	subscription := awaitValue(t, source.subscription, "MCP invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != protocol.TopicMCPChanged {
		t.Fatalf("MCP subscription = %+v", subscription)
	}
	host.Type("/mcp")
	host.Press(input.Enter)
	host.Shows(t, "Documentation")
	service.mu.Lock()
	service.servers[0].Description = "Updated documentation server"
	service.mu.Unlock()
	source.events <- changefeed.Event{Type: protocol.RuntimeMCPChanged, Sequence: 1, ServerIDs: []string{"docs"}}
	awaitSignal(t, source.applied, "mcp.changed delivery")
	host.Shows(t, "Updated documentation server")
	stop()
}

// The stub retains its catalog, so each query must transfer an independent result.
func cloneMCPServer(server protocol.MCPServer) protocol.MCPServer {
	server.Connection.Args = slices.Clone(server.Connection.Args)
	server.Connection.HeadersMasked = maps.Clone(server.Connection.HeadersMasked)
	server.Connection.EnvMasked = maps.Clone(server.Connection.EnvMasked)
	server.DisabledTools = slices.Clone(server.DisabledTools)
	server.AutoApproveTools = slices.Clone(server.AutoApproveTools)
	if server.HandshakeTimeout.Seconds != nil {
		server.HandshakeTimeout.Seconds = new(*server.HandshakeTimeout.Seconds)
	}
	if server.Status.ToolCount != nil {
		server.Status.ToolCount = new(*server.Status.ToolCount)
	}
	server.Status.Error = failure.Clone(server.Status.Error)
	return server
}
