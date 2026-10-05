package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func installationServer(t *testing.T, name string) mcpserver.Server {
	t.Helper()
	installation, err := resourceid.ParseInstallation("8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746")
	if err != nil {
		t.Fatal(err)
	}
	source, err := mcpserver.InstallationSource(installation, testsupport.Digest("release"), testsupport.Digest("authority"), testsupport.Digest("recipient"))
	if err != nil {
		t.Fatal(err)
	}
	return mcpserver.Server{Source: source, Name: testsupport.ServerName(name), Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-" + name}
}

// blockingConfigure holds a connection attempt that read its configuration
// before an installation change committed.
type blockingConfigure struct {
	fakePorts
	attempt chan context.Context
}

func (b *blockingConfigure) Configure(ctx context.Context, _ mcpserver.ID) error {
	b.attempt <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestInstallationWithdrawalSupersedesInFlightConnectionSynchronously(t *testing.T) {
	server := installationServer(t, "files")
	registry := &testRegistry{servers: map[mcpserver.ID]mcpserver.Server{server.ID(): server}}
	live := &blockingConfigure{attempt: make(chan context.Context, 1)}
	c := testCoordinator(t, Config{Registry: registry, ConnectionLifecycle: live})
	defer requireCoordinatorShutdown(t, c)
	if err := c.ReconcileInstallation(t.Context(), []mcpserver.ID{server.ID()}); err != nil {
		t.Fatal(err)
	}
	attempt := <-live.attempt

	if err := c.WithdrawInstallation([]mcpserver.ID{server.ID()}); err != nil {
		t.Fatal(err)
	}
	if live.removeName != server.Name.String() {
		t.Fatalf("withdrawal returned before detaching the source: %q", live.removeName)
	}
	if attempt.Err() == nil {
		t.Fatal("withdrawal returned while a superseded connection attempt could still dial")
	}
}

func TestInstallationWithdrawalRefusesUserSourcesWithoutEffect(t *testing.T) {
	live := &fakePorts{}
	c := testCoordinator(t, Config{ConnectionLifecycle: live})
	err := c.WithdrawInstallation([]mcpserver.ID{installationServer(t, "files").ID(), testsupport.UserMCPServer("files")})
	if !errors.Is(err, ErrInvalidServerConfiguration) || live.removeName != "" {
		t.Fatalf("withdraw user source = %v, detached %q", err, live.removeName)
	}
}

// Withdrawal retires the previous connection inside the change. A
// reconciliation that cannot start must leave that outcome at the status
// owner, not only in a log.
func TestUnrealizedInstallationSourceIsVisibleAtConnectionStatus(t *testing.T) {
	enabled := installationServer(t, "files")
	removed := installationServer(t, "retired")
	registry := &testRegistry{servers: map[mcpserver.ID]mcpserver.Server{enabled.ID(): enabled}}
	live := &fakePorts{}
	c := testCoordinator(t, Config{Registry: registry, StatusReader: live, ConnectionLifecycle: live})
	requireCoordinatorShutdown(t, c)

	if err := c.ReconcileInstallation(t.Context(), []mcpserver.ID{enabled.ID(), removed.ID()}); !errors.Is(err, errClosed) {
		t.Fatalf("reconcile after shutdown = %v, want closed", err)
	}
	status, err := c.ServerStatus(t.Context(), enabled.ID())
	if err != nil || !status.Known || status.State != mcpserver.ConnectionFailed || status.Failure != mcpserver.FailureConfiguration {
		t.Fatalf("unrealized source status = %+v, %v; want failed configuration", status, err)
	}
	if status, err := c.ServerStatus(t.Context(), removed.ID()); err != nil || status.Known {
		t.Fatalf("removed source became a failed ghost: %+v, %v", status, err)
	}
}

func TestToolsPairTheCatalogWithItsOwnConflicts(t *testing.T) {
	read := mcpserver.AdvertisedTool{Server: testsupport.UserMCPServer("a_b"), Name: testsupport.RemoteToolName("c")}
	clash := mcpserver.AdvertisedTool{Server: testsupport.UserMCPServer("a"), Name: testsupport.RemoteToolName("b_c")}
	readRef, clashRef := testMCPRef(read.Server, read.Name), testMCPRef(clash.Server, clash.Name)
	ports := &fakePorts{
		tools:     []mcpserver.AdvertisedTool{read, clash},
		conflicts: map[tool.Ref][]tool.Ref{readRef: {clashRef}, clashRef: {readRef}},
	}
	c := testCoordinator(t, Config{ToolCatalog: ports})
	views, err := c.Tools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ports.toolsCalls != 1 || len(views) != 2 {
		t.Fatalf("catalog reads = %d, views = %+v; want one snapshot", ports.toolsCalls, views)
	}
	for _, view := range views {
		if len(view.Conflicts) != 1 {
			t.Fatalf("view %s lost its snapshot conflicts: %+v", view.ModelName, view.Conflicts)
		}
	}
}

// failingDefinitions is a registry whose source owner cannot be read.
type failingDefinitions struct{ *testRegistry }

var errSourceUnreadable = errors.New("source owner unreadable")

func (failingDefinitions) Definition(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return mcpserver.Server{}, false, errSourceUnreadable
}

// A reconciliation that cannot read a committed source refuses it, as a
// dispatch does: withdrawal already retired its connection, and reading as
// merely disconnected would hide that nothing will dial it.
func TestUnreadableInstallationSourceIsRefusedAtConnectionStatus(t *testing.T) {
	server := installationServer(t, "files")
	live := &fakePorts{}
	c := testCoordinator(t, Config{Registry: failingDefinitions{&testRegistry{}}, StatusReader: live, ConnectionLifecycle: live})
	defer requireCoordinatorShutdown(t, c)

	if err := c.ReconcileInstallation(t.Context(), []mcpserver.ID{server.ID()}); !errors.Is(err, errSourceUnreadable) {
		t.Fatalf("reconcile an unreadable source = %v, want the read failure", err)
	}
	status, err := c.ServerStatus(t.Context(), server.ID())
	if err != nil || !status.Known || status.State != mcpserver.ConnectionFailed || status.Failure != mcpserver.FailureConfiguration {
		t.Fatalf("unreadable source status = %+v, %v; want failed configuration", status, err)
	}
}
