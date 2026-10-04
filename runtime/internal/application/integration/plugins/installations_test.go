package plugins

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/google/uuid"
)

type installationMemory struct{ record *plugin.Record }

func (s *installationMemory) List(context.Context) ([]*plugin.Installation, error) {
	if s.record == nil {
		return nil, nil
	}
	i, err := plugin.Restore(*s.record)
	if err != nil {
		return nil, err
	}
	return []*plugin.Installation{i}, nil
}
func (s *installationMemory) Get(context.Context, string) (*plugin.Installation, error) {
	if s.record == nil {
		return nil, plugin.ErrNotFound
	}
	return plugin.Restore(*s.record)
}
func (s *installationMemory) Save(_ context.Context, i *plugin.Installation) error {
	r := i.Snapshot()
	s.record = &r
	return nil
}
func (s *installationMemory) Remove(context.Context, string) error { s.record = nil; return nil }

type installationPackages struct {
	prepare func(context.Context, *plugin.Installation) ([]plugin.Diagnostic, error)
	servers func(context.Context, *plugin.Installation) (Backends, error)
}

func (installationPackages) Materialize(context.Context, string) (plugin.Release, error) {
	return plugin.Release{}, errors.New("unexpected materialization")
}
func (p installationPackages) Prepare(ctx context.Context, installation *plugin.Installation) ([]plugin.Diagnostic, error) {
	if p.prepare != nil {
		return p.prepare(ctx, installation)
	}
	return nil, nil
}
func (p installationPackages) Servers(ctx context.Context, i *plugin.Installation) (Backends, error) {
	if p.servers != nil {
		return p.servers(ctx, i)
	}
	r := i.Snapshot()
	name, err := mcpserver.InstallationServer(r.ID, "server")
	if err != nil {
		return Backends{}, err
	}
	return Backends{Servers: []mcpserver.Server{{Name: name, Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp", Enabled: i.ServerEnabled("server"), ReleaseAuthority: i.ServerAuthority("server")}}}, nil
}

type installationDependencies struct{ held bool }

func (d *installationDependencies) ChangeInstallation(_ context.Context, _ string, _ ChangeAdmission, change func() error) error {
	d.held = true
	defer func() { d.held = false }()
	return change()
}

type installationConnections struct {
	reconcile func(context.Context, []mcpserver.ServerName) error
}

func (c installationConnections) ReconcileInstallation(ctx context.Context, names []mcpserver.ServerName) error {
	return c.reconcile(ctx, names)
}

func installationFixture(t *testing.T) (*installationMemory, *installationDependencies, string) {
	t.Helper()
	release := plugin.Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []plugin.Server{{Name: "server", Type: "streamable-http", URL: "https://example.test/mcp"}}}
	i, err := plugin.New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	s := &installationMemory{}
	if err := s.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	return s, &installationDependencies{}, i.Snapshot().ID
}

func TestCommittedInstallationReportsRealizationFailure(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ServerName) error {
		if dependencies.held {
			t.Fatal("external reconcile retained executable admission")
		}
		current, err := store.Get(ctx, id)
		if err != nil || !current.Active() {
			t.Fatal("realization preceded durable enablement")
		}
		return errors.New("supervisor is closing")
	}}
	c, err := New(t.Context(), store, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Enable(t.Context(), id, true)
	if err != nil || !result.Record.Enabled || len(result.Availability) != 1 || result.Availability[0].Code != "reconciliation_failed" {
		t.Fatalf("committed result was reported as failed command: %+v, %v", result, err)
	}
}

func TestListingPreservesCancellationInsteadOfProjectingUnavailability(t *testing.T) {
	store, dependencies, _ := installationFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	cause := errors.New("listing abandoned")
	packages := installationPackages{servers: func(ctx context.Context, _ *plugin.Installation) (Backends, error) {
		cancel(cause)
		return Backends{}, errors.Join(plugin.ErrUnavailable, ctx.Err())
	}}
	c, err := New(t.Context(), store, packages, installationConnections{}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	inspections, err := c.List(ctx)
	if !errors.Is(err, cause) || len(inspections) != 0 {
		t.Fatalf("canceled listing = %+v, %v; want original cancellation cause", inspections, err)
	}
}

func TestListingReportsUnavailableReleaseWithoutLosingTheInstallation(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	packages := installationPackages{servers: func(context.Context, *plugin.Installation) (Backends, error) {
		return Backends{}, plugin.ErrUnavailable
	}}
	c, err := New(t.Context(), store, packages, installationConnections{}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	inspections, err := c.List(t.Context())
	if err != nil || len(inspections) != 1 {
		t.Fatalf("unavailable listing = %+v, %v", inspections, err)
	}
	if inspections[0].Record.ID != id || !reflect.DeepEqual(inspections[0].Availability, []plugin.Diagnostic{{Component: "release", Code: "unavailable_release"}}) {
		t.Fatalf("unavailable listing lost the installation or its diagnostic: %+v", inspections[0])
	}
}

func TestStaleConfigurationNeverReachesPersistenceOrRealization(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	before, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(t.Context(), store, installationPackages{prepare: func(context.Context, *plugin.Installation) ([]plugin.Diagnostic, error) {
		t.Fatal("stale configuration reached preparation")
		return nil, nil
	}}, installationConnections{reconcile: func(context.Context, []mcpserver.ServerName) error {
		t.Fatal("stale configuration reached reconciliation")
		return nil
	}}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Configure(t.Context(), id, plugin.Configuration{Digest: strings.Repeat("2", 64), DisabledServers: []string{"server"}}); !errors.Is(err, plugin.ErrStale) {
		t.Fatalf("stale configuration = %v", err)
	}
	after, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Snapshot(), after.Snapshot()) {
		t.Fatal("stale configuration advanced durable installation facts")
	}
}

func TestPreparationReportsFailureAfterDurableAdmission(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	packages := installationPackages{prepare: func(ctx context.Context, installation *plugin.Installation) ([]plugin.Diagnostic, error) {
		if dependencies.held {
			t.Fatal("package preparation retained executable admission")
		}
		current, err := store.Get(ctx, id)
		if err != nil || !current.Active() || !installation.Active() {
			t.Fatal("package preparation preceded durable enablement")
		}
		return []plugin.Diagnostic{{Component: "mcp:server", Code: "preparation_failed"}}, errors.New("package root is unavailable")
	}}
	reconciled := false
	connections := installationConnections{reconcile: func(context.Context, []mcpserver.ServerName) error {
		reconciled = true
		return nil
	}}
	c, err := New(t.Context(), store, packages, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Enable(t.Context(), id, true)
	if err != nil || !result.Record.Enabled || !reconciled || len(result.Availability) != 2 {
		t.Fatalf("committed preparation diagnostics = %+v, %v", result, err)
	}
	for _, diagnostic := range result.Availability {
		if diagnostic.Code != "preparation_failed" {
			t.Fatalf("unexpected preparation diagnostic: %+v", diagnostic)
		}
	}
}

func TestUninstallWithdrawsDurablyBeforeFailedCleanup(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ServerName) error {
		if dependencies.held {
			t.Fatal("cleanup retained executable admission")
		}
		if _, err := store.Get(ctx, id); !errors.Is(err, plugin.ErrNotFound) {
			t.Fatalf("uninstalled source can be restored: %v", err)
		}
		return errors.New("retirement failed")
	}}
	c, err := New(t.Context(), store, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Uninstall(t.Context(), id)
	if err != nil || len(result.Availability) != 1 {
		t.Fatalf("durable removal lost cleanup diagnostic: %+v, %v", result, err)
	}
}

func TestInstallationReconcileCanReenterTheOwner(t *testing.T) {
	store, dependencies, id := installationFixture(t)
	var c *Coordinator
	calls := 0
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ServerName) error {
		calls++
		if calls == 1 {
			_, err := c.Enable(ctx, id, false)
			return err
		}
		return nil
	}}
	var err error
	c, err = New(t.Context(), store, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Enable(t.Context(), id, true); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(t.Context(), id)
	if err != nil || current.Active() || calls != 2 {
		t.Fatalf("reentrant withdrawal: %+v, %v, calls=%d", current, err, calls)
	}
}

func TestCommittedInstallationRealizationFollowsRuntimeLifetime(t *testing.T) {
	for _, phase := range []string{"preparation", "reconciliation", "projection", "removal"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store, dependencies, id := installationFixture(t)
				lifetime, stopRuntime := context.WithCancelCause(t.Context())
				defer stopRuntime(nil)
				type traceKey struct{}
				request, cancelRequest := context.WithCancel(context.WithValue(t.Context(), traceKey{}, "installation change"))
				defer cancelRequest()
				started := make(chan context.Context, 1)
				release := make(chan struct{})
				wait := func(ctx context.Context) error {
					started <- ctx
					select {
					case <-ctx.Done():
						return context.Cause(ctx)
					case <-release:
						return nil
					}
				}
				packages := installationPackages{}
				connections := installationConnections{reconcile: func(context.Context, []mcpserver.ServerName) error { return nil }}
				switch phase {
				case "preparation":
					packages.prepare = func(ctx context.Context, _ *plugin.Installation) ([]plugin.Diagnostic, error) { return nil, wait(ctx) }
				case "reconciliation", "removal":
					connections.reconcile = func(ctx context.Context, _ []mcpserver.ServerName) error { return wait(ctx) }
				case "projection":
					packages.servers = func(ctx context.Context, _ *plugin.Installation) (Backends, error) { return Backends{}, wait(ctx) }
				}
				c, err := New(lifetime, store, packages, connections, dependencies, nil)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() {
					if phase == "removal" {
						_, err := c.Uninstall(request, id)
						done <- err
						return
					}
					_, err := c.Enable(request, id, true)
					done <- err
				}()
				synctest.Wait()
				operation := <-started
				cancelRequest()
				synctest.Wait()
				if operation.Err() != nil || operation.Value(traceKey{}) != "installation change" {
					close(release)
					synctest.Wait()
					t.Fatalf("committed realization lost request independence or trace values: %v", operation.Err())
				}
				cause := errors.New("runtime stopped")
				stopRuntime(cause)
				synctest.Wait()
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("committed mutation reported failure: %v", err)
					}
				default:
					close(release)
					synctest.Wait()
					t.Fatal("runtime shutdown could not retire committed realization")
				}
				if !errors.Is(context.Cause(operation), cause) {
					t.Fatalf("realization cancellation = %v, want runtime cause", context.Cause(operation))
				}
				if phase == "removal" {
					if store.record != nil {
						t.Fatal("shutdown restored removed admission")
					}
				} else if store.record == nil || !store.record.Enabled {
					t.Fatal("shutdown reversed durable enablement")
				}
			})
		})
	}
}
