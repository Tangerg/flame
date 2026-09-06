package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestServersAndToolsUsePorts(t *testing.T) {
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{
			{Name: testMCPServerName("fs"), State: mcpserver.ConnectionConnected, ToolCount: 1},
			{Name: testMCPServerName("docs"), State: mcpserver.ConnectionFailed},
		},
		tools: []mcpserver.AdvertisedTool{{Server: testMCPServerName("fs"), Name: testRemoteToolName("read")}},
	}
	c := testCoordinator(t, configWithPorts(ports))

	if got, err := c.Servers(context.Background()); err != nil || len(got) != 2 ||
		got[0].Name.String() != "docs" || got[1].Name.String() != "fs" ||
		got[1].State.ToolCount == nil || *got[1].State.ToolCount != 1 {
		t.Fatalf("Servers = %+v, %v", got, err)
	}
	if ports.toolsCalls != 0 {
		t.Fatalf("status read made %d live tools/list calls, want 0", ports.toolsCalls)
	}
	name := testMCPServerName("fs")
	tools, err := c.Tools(context.Background(), &name)
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	if ports.toolsCalls != 1 || ports.toolsServer != "fs" || len(tools) != 1 || tools[0].Name.String() != "read" {
		t.Fatalf("tools server=%q tools=%+v", ports.toolsServer, tools)
	}
}

func TestServersDoNotExposeStoredMutableValues(t *testing.T) {
	name := testMCPServerName("files")
	server := mcpserver.Server{
		Name: name, Enabled: true, Transport: mcpserver.TransportStdio,
		Command: "mcp-files", Args: []string{"--root", "/repo"},
	}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{name: server}}
	ports := &fakePorts{}
	coordinator := testCoordinator(t, Config{Registry: registry, StatusReader: ports})

	servers, err := coordinator.Servers(t.Context())
	if err != nil || len(servers) != 1 {
		t.Fatalf("Servers = (%+v, %v), want one server", servers, err)
	}
	servers[0].Connection.Args[0] = "changed"
	if server.Args[0] != "--root" {
		t.Fatal("returned server changed stored arguments")
	}
}

func TestToolsOwnCatalogOrder(t *testing.T) {
	ports := &fakePorts{tools: []mcpserver.AdvertisedTool{
		{Server: testMCPServerName("zeta"), Name: testRemoteToolName("alpha")},
		{Server: testMCPServerName("alpha"), Name: testRemoteToolName("zeta")},
		{Server: testMCPServerName("alpha"), Name: testRemoteToolName("alpha")},
	}}
	c := testCoordinator(t, Config{ToolCatalog: ports})

	tools, err := c.Tools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 ||
		tools[0].Server.String() != "alpha" || tools[0].Name.String() != "alpha" ||
		tools[1].Server.String() != "alpha" || tools[1].Name.String() != "zeta" ||
		tools[2].Server.String() != "zeta" || tools[2].Name.String() != "alpha" {
		t.Fatalf("tools = %+v, want alpha/alpha, alpha/zeta, zeta/alpha", tools)
	}
	if ports.tools[0].Server.String() != "zeta" {
		t.Fatal("Tools reordered the catalog port's retained slice")
	}
}

func TestToolsRejectsBrokenOrOutOfScopeCatalogs(t *testing.T) {
	files := testMCPServerName("files")
	read := mcpserver.AdvertisedTool{Server: files, Name: testRemoteToolName("read")}
	for name, tools := range map[string][]mcpserver.AdvertisedTool{
		"invalid descriptor": {{}},
		"duplicate identity": {read, read},
		"foreign scope":      {{Server: testMCPServerName("other"), Name: testRemoteToolName("read")}},
	} {
		t.Run(name, func(t *testing.T) {
			c := testCoordinator(t, Config{ToolCatalog: &fakePorts{tools: tools}})
			if result, err := c.Tools(t.Context(), &files); err == nil || result != nil {
				t.Fatalf("Tools = (%+v, %v), want nil/error", result, err)
			}
		})
	}

	overCapacity := make([]mcpserver.AdvertisedTool, mcpserver.MaxRemoteToolsPerServer+1)
	for index := range overCapacity {
		overCapacity[index] = mcpserver.AdvertisedTool{
			Server: files, Name: testRemoteToolName(fmt.Sprintf("tool-%04d", index)),
		}
	}
	c := testCoordinator(t, Config{ToolCatalog: &fakePorts{tools: overCapacity}})
	if result, err := c.Tools(t.Context(), &files); err == nil || result != nil {
		t.Fatalf("over-capacity Tools = (%d rows, %v), want nil/error", len(result), err)
	}

	invalidScope := mcpserver.ServerName{}
	ports := &fakePorts{tools: []mcpserver.AdvertisedTool{read}}
	c = testCoordinator(t, Config{ToolCatalog: ports})
	if result, err := c.Tools(t.Context(), &invalidScope); err == nil || result != nil {
		t.Fatalf("invalid-scope Tools = (%+v, %v), want nil/error", result, err)
	}
	if ports.toolsCalls != 0 {
		t.Fatalf("invalid scope reached tool catalog %d times", ports.toolsCalls)
	}
}

func TestServersRejectsBrokenRegistryCatalog(t *testing.T) {
	server := mcpserver.Server{
		Name: testMCPServerName("files"), Enabled: true,
		Transport: mcpserver.TransportStdio, Command: "mcp-files",
	}
	for name, listed := range map[string][]mcpserver.Server{
		"invalid server":     {{}},
		"duplicate identity": {server, server},
	} {
		t.Run(name, func(t *testing.T) {
			c := testCoordinator(t, Config{Registry: &testRegistry{listed: listed}})
			if servers, err := c.Servers(t.Context()); err == nil || servers != nil {
				t.Fatalf("Servers = (%+v, %v), want nil/error", servers, err)
			}
		})
	}
}

func TestServersRejectsBrokenLiveStatusCatalog(t *testing.T) {
	name := testMCPServerName("files")
	server := mcpserver.Server{
		Name: name, Enabled: true,
		Transport: mcpserver.TransportStdio, Command: "mcp-files",
	}
	valid := mcpserver.ConnectionStatus{Name: name, State: mcpserver.ConnectionConnected, ToolCount: 1}
	for caseName, statuses := range map[string][]mcpserver.ConnectionStatus{
		"invalid status":     {{}},
		"duplicate identity": {valid, valid},
		"discarded count":    {{Name: name, State: mcpserver.ConnectionFailed, ToolCount: 1}},
	} {
		t.Run(caseName, func(t *testing.T) {
			ports := &fakePorts{statuses: statuses}
			c := testCoordinator(t, Config{
				Registry:     &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{name: server}},
				StatusReader: ports,
			})
			if servers, err := c.Servers(t.Context()); err == nil || servers != nil {
				t.Fatalf("Servers = (%+v, %v), want nil/error", servers, err)
			}
		})
	}
}

func TestServerStatusRejectsCorruptApplicationOverride(t *testing.T) {
	name := testMCPServerName("files")
	c := testCoordinator(t, Config{})
	c.statusOverrides[name] = ServerStatus{Name: name, Known: true, State: mcpserver.ConnectionConnected}

	if status, err := c.ServerStatus(t.Context(), name); err == nil || status != (ServerStatus{}) {
		t.Fatalf("ServerStatus = (%+v, %v), want zero/error", status, err)
	}
}

func TestServerStatusRejectsInvalidRequestedIdentity(t *testing.T) {
	c := testCoordinator(t, Config{StatusReader: &fakePorts{}})
	if status, err := c.ServerStatus(t.Context(), mcpserver.ServerName{}); err == nil || status != (ServerStatus{}) {
		t.Fatalf("ServerStatus = (%+v, %v), want zero/error", status, err)
	}
}

func TestMCPRegistryReadsRejectMismatchedIdentity(t *testing.T) {
	requested := testMCPServerName("requested")
	foreign := mcpserver.Server{
		Name: testMCPServerName("foreign"), Enabled: true,
		Transport: mcpserver.TransportStdio, Command: "mcp-foreign",
	}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{requested: foreign}}
	ports := &fakePorts{}
	c := testCoordinator(t, Config{
		Registry: registry, StatusReader: ports, ConnectionControl: ports,
		ConnectionLifecycle: ports,
	})

	if server, err := c.Server(t.Context(), requested); err == nil || server.Name.String() != "" {
		t.Fatalf("Server = (%+v, %v), want zero/error", server, err)
	}
	if err := c.ReconnectServer(t.Context(), requested); err == nil {
		t.Fatal("ReconnectServer accepted a mismatched registry row")
	}
	candidate := mcpserver.Server{
		Name: requested, Enabled: true,
		Transport: mcpserver.TransportStdio, Command: "mcp-requested",
	}
	if server, err := c.CreateServer(t.Context(), input(candidate)); err == nil || server.Name.String() != "" {
		t.Fatalf("CreateServer = (%+v, %v), want zero/error", server, err)
	}
	if result, err := c.TestServer(t.Context(), input(candidate)); err == nil || result != (TestResult{}) {
		t.Fatalf("TestServer = (%+v, %v), want zero/error", result, err)
	}
	enabled := false
	if server, err := c.UpdateServer(t.Context(), requested, ServerPatch{Enabled: &enabled}); err == nil || server.Name.String() != "" {
		t.Fatalf("UpdateServer = (%+v, %v), want zero/error", server, err)
	}
	if err := c.DeleteServer(t.Context(), requested); err == nil {
		t.Fatal("DeleteServer accepted a mismatched registry row")
	}
}

func TestBrokenRegistryCatalogCannotReplaceToolPolicy(t *testing.T) {
	server := mcpserver.Server{
		Name: testMCPServerName("files"), Enabled: true,
		Transport: mcpserver.TransportStdio, Command: "mcp-files",
	}
	policy := NewToolPolicyState(mcpserver.NewToolPolicy([]mcpserver.Server{server}))
	ref := mcpserver.ToolRef{Server: server.Name, Tool: testRemoteToolName("read")}
	c := testCoordinator(t, Config{Registry: &testRegistry{listed: []mcpserver.Server{{}}}, Policy: policy})

	if policy.ToolDisabled(ref) {
		t.Fatal("initial policy unexpectedly disabled configured server")
	}
	if err := c.refreshToolPolicy(t.Context()); err == nil {
		t.Fatal("refreshToolPolicy accepted a broken registry catalog")
	}
	if policy.ToolDisabled(ref) {
		t.Fatal("failed refresh replaced the last valid tool policy")
	}
}

func TestDeleteServerPublishesRemovalAfterProjectionFailure(t *testing.T) {
	projectionErr := errors.New("projection detach failed")
	ports := &fakePorts{
		statuses:  []mcpserver.ConnectionStatus{{Name: testMCPServerName("fs"), State: mcpserver.ConnectionConnected}},
		removeErr: projectionErr,
	}
	notified := make(chan string, 1)
	cfg := configWithPorts(ports)
	cfg.Invalidations = func(notice invalidation.Notice) { notified <- notice.ServerIDs[0] }
	c := testCoordinator(t, cfg)

	if err := c.DeleteServer(t.Context(), testMCPServerName("fs")); !errors.Is(err, projectionErr) {
		t.Fatalf("DeleteServer = %v, want projection failure", err)
	}
	if ports.removeName != "fs" {
		t.Fatalf("live removal = %q, want fs", ports.removeName)
	}
	if got := <-notified; got != "fs" {
		t.Fatalf("status notification = %q, want fs", got)
	}
}

// TestReconnectServerUsesPort: reconnect is fire-and-forget — it validates
// the name synchronously, then dials on the component task group and publishes
// the settled frame.
func TestReconnectServerUsesPort(t *testing.T) {
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: testMCPServerName("fs"), State: mcpserver.ConnectionConnected}}}
	settled := make(chan string, 1)
	var c *Coordinator
	cfg := configWithPorts(ports)
	cfg.Invalidations = func(notice invalidation.Notice) {
		status := testServerStatus(c, testMCPServerName(notice.ServerIDs[0]))
		if status.State != mcpserver.ConnectionConnecting {
			settled <- status.Name.String()
		}
	}
	c = testCoordinator(t, cfg)
	defer requireCoordinatorShutdown(t, c)

	if err := c.ReconnectServer(context.Background(), testMCPServerName("fs")); err != nil {
		t.Fatalf("ReconnectServer err = %v", err)
	}
	if got := <-settled; got != "fs" {
		t.Fatalf("settled server = %q, want fs", got)
	}
	if ports.reconnectName != "fs" {
		t.Fatalf("reconnect=%q, want fs", ports.reconnectName)
	}

	if err := c.ReconnectServer(context.Background(), testMCPServerName("ghost")); !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownServer", err)
	}
}

func TestAuthorizationAttemptUsesPortAndSettles(t *testing.T) {
	authorizeStarted := make(chan string, 1)
	releaseAuthorize := make(chan struct{})
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: testMCPServerName("github")}},
		authorizeStarted: authorizeStarted,
		releaseAuthorize: releaseAuthorize,
	}
	c := testCoordinator(t, configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	attempt, err := c.CreateAuthorizationAttempt(context.Background(), testMCPServerName("github"))
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	if attempt.ID.String() == "" || attempt.Server.String() != "github" || attempt.Status != AuthorizationAttemptPending ||
		attempt.CreatedAt.IsZero() || attempt.FinishedAt != nil {
		t.Fatalf("created attempt = %+v", attempt)
	}
	if got := <-authorizeStarted; got != "github" {
		t.Fatalf("authorization target = %q, want github", got)
	}
	close(releaseAuthorize)

	settled := awaitAuthorizationAttempt(t, c, attempt.ID)
	if settled.Status != AuthorizationAttemptSucceeded || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want succeeded", settled)
	}
	if ports.authorizeName != "github" {
		t.Fatalf("authorize=%q, want github", ports.authorizeName)
	}
}

func TestConnectionValidationUsesDurableRegistry(t *testing.T) {
	name := testMCPServerName("fs")
	ports := &fakePorts{reconnectDone: make(chan string, 1)}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	c := testCoordinator(t, Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	})

	// The live projection intentionally has no entry. Durable configuration is
	// the command authority; the background dial is what repairs that projection.
	if err := c.ReconnectServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectServer with stale live projection: %v", err)
	}
	if got := <-ports.reconnectDone; got != name.String() {
		t.Fatalf("reconnect target = %q, want %q", got, name)
	}
	requireCoordinatorShutdown(t, c)
	if ports.reconnectName != name.String() {
		t.Fatalf("reconnect target = %q, want %q", ports.reconnectName, name)
	}
}

func TestConnectionRejectsDurablyDisabledServer(t *testing.T) {
	name := testMCPServerName("fs")
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{
		name: {Name: name, Enabled: false, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	c := testCoordinator(t, Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	})

	// Even a stale connected projection cannot override the durable enablement
	// gate.
	if err := c.ReconnectServer(context.Background(), name); !errors.Is(err, ErrServerDisabled) {
		t.Fatalf("ReconnectServer = %v, want ErrServerDisabled", err)
	}
	if ports.reconnectName != "" {
		t.Fatalf("disabled server was dialed as %q", ports.reconnectName)
	}
}

func TestStatusCallbackMayReenterMutationWithoutDeadlock(t *testing.T) {
	name := testMCPServerName("fs")
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testRegistry{servers: map[mcpserver.ServerName]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	statuses := make(chan ServerStatus, 2)
	mutationResult := make(chan error, 1)
	cfg := Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
	var c *Coordinator
	cfg.Invalidations = func(notice invalidation.Notice) {
		status := testServerStatus(c, testMCPServerName(notice.ServerIDs[0]))
		statuses <- status
		if status.State == mcpserver.ConnectionConnecting {
			// A status consumer is application-external code. It may synchronously
			// issue another command; publication must hold neither mutation nor
			// delivery-ordering locks while invoking it.
			enabled := false
			_, err := c.UpdateServer(context.Background(), name, ServerPatch{Enabled: &enabled})
			mutationResult <- err
		}
	}
	c = testCoordinator(t, cfg)

	if err := c.ReconnectServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectServer: %v", err)
	}
	first := <-statuses
	if first.Name != name || first.State != mcpserver.ConnectionConnecting || !first.Known {
		t.Fatalf("first status = %+v, want connecting", first)
	}
	if err := <-mutationResult; err != nil {
		t.Fatalf("reentrant UpdateServer: %v", err)
	}
	second := <-statuses
	if second.Name != name || second.Known {
		t.Fatalf("second status = %+v, want ordered removal projection", second)
	}
	requireCoordinatorShutdown(t, c)

	server, ok, err := registry.Get(context.Background(), name)
	if err != nil || !ok || server.Enabled {
		t.Fatalf("durable server after reentrant disable = (%+v, %v, %v)", server, ok, err)
	}
}

func TestConnectionInvalidationReadsConnectingThenSettled(t *testing.T) {
	name := testMCPServerName("fs")
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected, ToolCount: 1}}}
	var c *Coordinator
	states := make(chan mcpserver.ConnectionState, 2)
	cfg := configWithPorts(ports)
	cfg.Invalidations = func(notice invalidation.Notice) {
		if notice.Resource != invalidation.MCP || len(notice.ServerIDs) != 1 || notice.ServerIDs[0] != name.String() {
			t.Fatalf("notice = %+v, want MCP/fs", notice)
		}
		states <- testServerStatus(c, name).State
	}
	c = testCoordinator(t, cfg)

	if err := c.ReconnectServer(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	got := []mcpserver.ConnectionState{<-states, <-states}
	if !slices.Equal(got, []mcpserver.ConnectionState{
		mcpserver.ConnectionConnecting,
		mcpserver.ConnectionConnected,
	}) {
		t.Fatalf("readable states at invalidation = %v, want [connecting connected]", got)
	}
	requireCoordinatorShutdown(t, c)

	// The terminal overlay is only a publication handoff. A later passive live
	// transition must remain visible even though no new connection command wrote
	// the application overlay.
	ports.statuses[0].State = mcpserver.ConnectionFailed
	ports.statuses[0].ToolCount = 0
	if status := testServerStatus(c, name); status.State != mcpserver.ConnectionFailed {
		t.Fatalf("passive live status = %+v, want failed", status)
	}
}

func testServerStatus(c *Coordinator, name mcpserver.ServerName) ServerStatus {
	status, err := c.ServerStatus(context.Background(), name)
	if err != nil {
		panic(err)
	}
	return status
}

// TestReconnectServerDetachedButComponentOwned: a dial detaches the caller's
// cancellation (a returning RPC must not abort it) while preserving its trace
// values. BeginShutdown cancels it, AwaitShutdown joins it, and a subsequent
// reconnect reports errClosed.
func TestReconnectServerDetachedButComponentOwned(t *testing.T) {
	type ctxKey struct{}
	ports := &blockingPorts{
		fakePorts: fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: testMCPServerName("fs")}}},
		started:   make(chan bool, 1),
		stopped:   make(chan struct{}),
		wantValue: func(ctx context.Context) bool { return ctx.Value(ctxKey{}) == "trace" },
	}
	c := testCoordinator(t, configWithPorts(ports))

	reqCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace"))
	cancelRequest() // the request is done — the dial must keep running

	if err := c.ReconnectServer(reqCtx, testMCPServerName("fs")); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if detached := <-ports.started; !detached {
		t.Fatal("dial context did not detach request cancellation or preserve values")
	}

	requireCoordinatorShutdown(t, c)
	select {
	case <-ports.stopped:
	case <-time.After(time.Second):
		t.Fatal("Coordinator.Close did not cancel and join the dial")
	}
	if err := c.ReconnectServer(context.Background(), testMCPServerName("fs")); !errors.Is(err, errClosed) {
		t.Fatalf("reconnect after Close = %v, want errClosed", err)
	}
}

func TestTestServerUsesLiveRegistryPort(t *testing.T) {
	ports := &fakePorts{}
	c := testCoordinator(t, configWithPorts(ports))

	result, err := c.TestServer(context.Background(), ServerInput{
		Name: testMCPServerName("fs"), Connection: ConnectionInput{
			Transport: mcpserver.TransportStdio, Command: "mcp-fs",
			Args:        []string{"--root", "/repo"},
			Environment: &EnvironmentChange{Kind: SecretSet, Value: map[string]string{"A": "1"}},
		},
	})
	if err != nil {
		t.Fatalf("TestServer err = %v", err)
	}
	if !result.OK {
		t.Fatalf("TestServer result = %+v, want success", result)
	}
	if ports.probe.Name.String() != "fs" || ports.probe.Command != "mcp-fs" || ports.probe.Env["A"] != "1" {
		t.Fatalf("probe config = %+v", ports.probe)
	}
}

func TestServerCommandsOwnInputsAfterReturning(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		name := testMCPServerName("create")
		serverInput := stdioServerInput(name, "original")
		registry := &testRegistry{
			servers: make(map[mcpserver.ServerName]mcpserver.Server),
		}

		if _, err := testCoordinator(t, Config{Registry: registry}).CreateServer(t.Context(), serverInput); err != nil {
			t.Fatal(err)
		}
		serverInput.Connection.Args[0] = "caller-changed"
		serverInput.Connection.Environment.Value["TOKEN"] = "caller-changed"
		stored, found, err := registry.Get(t.Context(), name)
		if err != nil || !found {
			t.Fatalf("stored server = (%+v, %t, %v)", stored, found, err)
		}
		if stored.Args[0] != "original" || stored.Env["TOKEN"] != "original" {
			t.Fatalf("created server changed after the caller reused its input: %+v", stored)
		}
	})

	t.Run("update", func(t *testing.T) {
		name := testMCPServerName("update")
		current := mcpserver.Server{
			Name: name, Transport: mcpserver.TransportStdio, Command: "mcp-files",
		}
		description := "original description"
		connection := stdioServerInput(name, "original").Connection
		disabled := []mcpserver.RemoteToolName{testRemoteToolName("read")}
		patch := ServerPatch{Description: &description, Connection: &connection, DisabledTools: &disabled}
		registry := &testRegistry{
			servers: map[mcpserver.ServerName]mcpserver.Server{name: current},
		}

		if _, err := testCoordinator(t, Config{Registry: registry}).UpdateServer(t.Context(), name, patch); err != nil {
			t.Fatal(err)
		}
		description = "caller-changed"
		connection.Args[0] = "caller-changed"
		connection.Environment.Value["TOKEN"] = "caller-changed"
		disabled[0] = testRemoteToolName("changed")
		stored, found, err := registry.Get(t.Context(), name)
		if err != nil || !found {
			t.Fatalf("stored server = (%+v, %t, %v)", stored, found, err)
		}
		if stored.Description != "original description" || stored.Args[0] != "original" ||
			stored.Env["TOKEN"] != "original" ||
			!slices.Equal(stored.ToolPolicy.DisabledTools(), []mcpserver.RemoteToolName{testRemoteToolName("read")}) {
			t.Fatalf("updated server changed after the caller reused its input: %+v", stored)
		}
	})

	t.Run("test", func(t *testing.T) {
		name := testMCPServerName("test")
		serverInput := stdioServerInput(name, "original")
		registry := &testRegistry{
			servers: make(map[mcpserver.ServerName]mcpserver.Server),
		}
		ports := &fakePorts{}

		result, err := testCoordinator(t, Config{Registry: registry, ConnectionLifecycle: ports}).TestServer(t.Context(), serverInput)
		if err != nil || !result.OK {
			t.Fatalf("TestServer = (%+v, %v), want success", result, err)
		}
		serverInput.Connection.Args[0] = "caller-changed"
		serverInput.Connection.Environment.Value["TOKEN"] = "caller-changed"
		if ports.probe.Args[0] != "original" || ports.probe.Env["TOKEN"] != "original" {
			t.Fatalf("tested server changed after the caller reused its input: %+v", ports.probe)
		}
	})
}

func stdioServerInput(name mcpserver.ServerName, mutableValue string) ServerInput {
	return ServerInput{
		Name: name,
		Connection: ConnectionInput{
			Transport: mcpserver.TransportStdio, Command: "mcp-files",
			Args: []string{mutableValue},
			Environment: &EnvironmentChange{
				Kind: SecretSet, Value: map[string]string{"TOKEN": mutableValue},
			},
		},
	}
}

func TestCreateServerSeparatesDurableAndLiveConnectionOwnership(t *testing.T) {
	configured := make(chan struct{})
	ports := &fakePorts{configureDone: configured}
	cfg := configWithPorts(ports)
	registry := cfg.Registry.(*testRegistry)
	coordinator := testCoordinator(t, cfg)
	name := testMCPServerName("files")
	serverInput := ServerInput{
		Name: name, Enabled: true,
		Connection: ConnectionInput{
			Transport: mcpserver.TransportStdio, Command: "mcp-files",
			Args: []string{"--root", "/repo"},
			Environment: &EnvironmentChange{
				Kind: SecretSet, Value: map[string]string{"TOKEN": "original"},
			},
		},
	}

	if _, err := coordinator.CreateServer(t.Context(), serverInput); err != nil {
		t.Fatal(err)
	}
	serverInput.Connection.Args[0] = "caller-changed"
	serverInput.Connection.Environment.Value["TOKEN"] = "caller-changed"
	select {
	case <-configured:
	case <-time.After(time.Second):
		t.Fatal("live configuration did not run")
	}
	stored, found, err := registry.Get(t.Context(), name)
	if err != nil || !found {
		t.Fatalf("stored server = (%+v, %t, %v)", stored, found, err)
	}
	if stored.Args[0] != "--root" || stored.Env["TOKEN"] != "original" {
		t.Fatalf("caller changed durable server: %+v", stored)
	}
	if ports.configure.Args[0] != "--root" || ports.configure.Env["TOKEN"] != "original" {
		t.Fatalf("caller changed live configuration: %+v", ports.configure)
	}
}

type fakePorts struct {
	statuses []mcpserver.ConnectionStatus
	tools    []mcpserver.AdvertisedTool

	toolsServer string
	toolsCalls  int

	reconnectName    string
	reconnectDone    chan string
	authorizeName    string
	authorizeStarted chan string
	releaseAuthorize chan struct{}
	authorizeErr     error

	probe         mcpserver.Server
	configure     mcpserver.Server
	configureDone chan struct{}
	removeName    string
	removeErr     error
}

func (f *fakePorts) Statuses() []mcpserver.ConnectionStatus {
	return slices.Clone(f.statuses)
}

func (f *fakePorts) Tools(_ context.Context, server *mcpserver.ServerName) ([]mcpserver.AdvertisedTool, error) {
	f.toolsCalls++
	if server != nil {
		f.toolsServer = server.String()
	}
	return slices.Clone(f.tools), nil
}

func (f *fakePorts) Reconnect(_ context.Context, name mcpserver.ServerName) error {
	f.reconnectName = name.String()
	if f.reconnectDone != nil {
		f.reconnectDone <- name.String()
	}
	return nil
}

func (f *fakePorts) Authorize(ctx context.Context, name mcpserver.ServerName) error {
	f.authorizeName = name.String()
	if f.authorizeStarted != nil {
		f.authorizeStarted <- name.String()
	}
	if f.releaseAuthorize != nil {
		select {
		case <-f.releaseAuthorize:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for index := range f.statuses {
		if f.statuses[index].Name != name {
			continue
		}
		if f.authorizeErr != nil {
			f.statuses[index].State = mcpserver.ConnectionFailed
		} else {
			f.statuses[index].State = mcpserver.ConnectionConnected
		}
	}
	return f.authorizeErr
}

func (f *fakePorts) Probe(_ context.Context, cfg mcpserver.Server) error {
	f.probe = cfg.Clone()
	return nil
}

func (f *fakePorts) Configure(_ context.Context, cfg mcpserver.Server) error {
	f.configure = cfg.Clone()
	if f.configureDone != nil {
		close(f.configureDone)
	}
	return nil
}

func (f *fakePorts) Detach(name mcpserver.ServerName) error {
	f.removeName = name.String()
	return f.removeErr
}

// blockingPorts is a fakePorts whose dial blocks on its context until Close,
// so a test can observe the detach + component-owned-cancellation contract.
type blockingPorts struct {
	fakePorts
	started   chan bool
	stopped   chan struct{}
	wantValue func(context.Context) bool
}

func (b *blockingPorts) Reconnect(ctx context.Context, _ mcpserver.ServerName) error {
	b.started <- ctx.Err() == nil && b.wantValue(ctx)
	<-ctx.Done()
	close(b.stopped)
	return ctx.Err()
}

func configWithPorts(ports interface {
	StatusReader
	ToolCatalog
	ConnectionControl
	ConnectionLifecycle
}) Config {
	registry := &testRegistry{servers: make(map[mcpserver.ServerName]mcpserver.Server)}
	for _, status := range ports.Statuses() {
		registry.servers[status.Name] = mcpserver.Server{
			Name: status.Name, Enabled: true,
			Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/" + status.Name.String(),
		}
	}
	return Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
}

func awaitAuthorizationAttempt(t *testing.T, c *Coordinator, id AuthorizationAttemptID) AuthorizationAttempt {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		attempt, err := c.AuthorizationAttempt(context.Background(), id.String())
		if err != nil {
			t.Fatalf("AuthorizationAttempt: %v", err)
		}
		if attempt.Status != AuthorizationAttemptPending {
			return attempt
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("MCP authorization attempt %q did not settle", id.String())
	return AuthorizationAttempt{}
}

// testRegistry is a concurrency-safe registry fake. It deliberately returns
// reverse name order so Application-owned catalog order cannot accidentally
// depend on an adapter. Optional mutation hooks let concurrency tests stop a
// write after its durable commit.
type testRegistry struct {
	mu              sync.Mutex
	servers         map[mcpserver.ServerName]mcpserver.Server
	listed          []mcpserver.Server
	saveCommitted   chan struct{}
	releaseSave     chan struct{}
	removeCommitted chan struct{}
	releaseRemove   chan struct{}
}

func (t *testRegistry) List(context.Context) ([]mcpserver.Server, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.listed != nil {
		servers := make([]mcpserver.Server, len(t.listed))
		for index, server := range t.listed {
			servers[index] = server.Clone()
		}
		return servers, nil
	}
	servers := make([]mcpserver.Server, 0, len(t.servers))
	for _, server := range t.servers {
		servers = append(servers, server.Clone())
	}
	slices.SortFunc(servers, func(a, b mcpserver.Server) int {
		return cmp.Compare(b.Name.String(), a.Name.String())
	})
	return servers, nil
}

func (t *testRegistry) Get(_ context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	server, ok := t.servers[name]
	return server.Clone(), ok, nil
}

func (t *testRegistry) Save(_ context.Context, server mcpserver.Server) error {
	t.mu.Lock()
	t.servers[server.Name] = server.Clone()
	t.mu.Unlock()
	if t.saveCommitted != nil {
		close(t.saveCommitted)
	}
	if t.releaseSave != nil {
		<-t.releaseSave
	}
	return nil
}

func (t *testRegistry) Remove(_ context.Context, name mcpserver.ServerName) error {
	t.mu.Lock()
	delete(t.servers, name)
	t.mu.Unlock()
	if t.removeCommitted != nil {
		close(t.removeCommitted)
	}
	if t.releaseRemove != nil {
		<-t.releaseRemove
	}
	return nil
}
