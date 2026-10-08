package plugins

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type installationMemory struct {
	record  *plugin.Record
	catalog releaseMemory
}

func (s *installationMemory) List(ctx context.Context) ([]plugin.Snapshot, error) {
	if s.record == nil {
		return nil, nil
	}
	snapshot, err := s.Get(ctx, s.record.ID)
	if err != nil {
		return nil, err
	}
	return []plugin.Snapshot{snapshot}, nil
}
func (s *installationMemory) Get(ctx context.Context, _ resourceid.InstallationID) (plugin.Snapshot, error) {
	if s.record == nil {
		return plugin.Snapshot{}, plugin.ErrNotFound
	}
	installation, err := plugin.Restore(*s.record)
	if err != nil {
		return plugin.Snapshot{}, err
	}
	selected, err := s.catalog.Get(ctx, installation.Selected())
	if err != nil {
		return plugin.Snapshot{}, err
	}
	snapshot := plugin.Snapshot{Installation: installation, Selected: selected}
	if digest, staged := installation.Staged(); staged {
		staged, err := s.catalog.Get(ctx, digest)
		if err != nil {
			return plugin.Snapshot{}, err
		}
		snapshot.Staged = &staged
	}
	return snapshot, nil
}
func (s *installationMemory) Save(_ context.Context, i *plugin.Installation) error {
	r := i.Snapshot()
	s.record = &r
	return nil
}
func (s *installationMemory) Remove(context.Context, resourceid.InstallationID) error {
	s.record = nil
	return nil
}

type releaseMemory map[fingerprint.Digest]plugin.Release

func (m releaseMemory) Get(_ context.Context, digest fingerprint.Digest) (plugin.Release, error) {
	release, found := m[digest]
	if !found {
		return plugin.Release{}, plugin.ErrNotFound
	}
	return release, nil
}

type installationPackages struct {
	materialize func(context.Context, string) (plugin.Release, error)
	prepare     func(context.Context, *plugin.Installation) error
	realize     func(context.Context, *plugin.Installation) (Realization, error)
}

func (installationPackages) ReadView(context.Context, *plugin.Installation, plugin.ViewDeclaration) (string, error) {
	return "", errors.New("unexpected view read")
}

type candidateRelease struct{ release plugin.Release }

func (c candidateRelease) Release() plugin.Release                         { return c.release }
func (c candidateRelease) Publish(context.Context) (plugin.Release, error) { return c.release, nil }
func (candidateRelease) Discard() error                                    { return nil }

func (p installationPackages) Materialize(ctx context.Context, source string) (Candidate, error) {
	if p.materialize == nil {
		return nil, errors.New("unexpected materialization")
	}
	release, err := p.materialize(ctx, source)
	if err != nil {
		return nil, err
	}
	return candidateRelease{release: release}, nil
}
func (installationPackages) Reclaim(context.Context, []fingerprint.Digest) error { return nil }
func (p installationPackages) Prepare(ctx context.Context, installation *plugin.Installation, _ plugin.Release) error {
	if p.prepare != nil {
		return p.prepare(ctx, installation)
	}
	return nil
}
func (p installationPackages) Realize(ctx context.Context, i *plugin.Installation, _ plugin.Release) (Realization, error) {
	if p.realize != nil {
		return p.realize(ctx, i)
	}
	if cause := context.Cause(ctx); cause != nil {
		return Realization{}, cause
	}
	return Realization{Release: ReleaseAvailable}, nil
}

type installationDependencies struct {
	held  bool
	inUse bool
}

func (d *installationDependencies) ChangeInstallation(ctx context.Context, _ resourceid.InstallationID, admission ChangeAdmission, change func([]plugin.Dependency) error) error {
	if d.inUse && admission == RequireQuiescent {
		return plugin.ErrInUse
	}
	return d.UnderAdmission(ctx, change)
}

func (d *installationDependencies) UnderAdmission(_ context.Context, step func([]plugin.Dependency) error) error {
	d.held = true
	defer func() { d.held = false }()
	return step(nil)
}

type admittedInstallationStore struct {
	*installationMemory
	dependencies *installationDependencies
}

func (s admittedInstallationStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	if !s.dependencies.held {
		return nil, errors.New("capacity read outside installation admission")
	}
	return s.installationMemory.List(ctx)
}

func (s admittedInstallationStore) Save(ctx context.Context, i *plugin.Installation) error {
	if !s.dependencies.held {
		return errors.New("installation write outside installation admission")
	}
	return s.installationMemory.Save(ctx, i)
}

type installationConnections struct {
	withdraw  func([]mcpserver.ID) error
	reconcile func(context.Context, []mcpserver.ID) error
}

func (c installationConnections) WithdrawInstallation(names []mcpserver.ID) error {
	if c.withdraw == nil {
		return nil
	}
	return c.withdraw(names)
}

func (c installationConnections) ReconcileInstallation(ctx context.Context, names []mcpserver.ID) error {
	return c.reconcile(ctx, names)
}

func installationFixture(t *testing.T) (*installationMemory, *installationDependencies, resourceid.InstallationID, releaseMemory) {
	t.Helper()
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"}}})
	i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	catalog := releaseMemory{release.Digest(): release}
	s := &installationMemory{catalog: catalog}
	if err := s.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	return s, &installationDependencies{}, i.ID(), catalog
}

func TestCommittedInstallationLeavesConnectionOutcomeToItsSupervisor(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ID) error {
		if dependencies.held {
			t.Fatal("external reconcile retained executable admission")
		}
		current, err := store.Get(ctx, id)
		if err != nil || !current.Installation.Active() {
			t.Fatal("realization preceded durable enablement")
		}
		return errors.New("supervisor is closing")
	}}
	c, err := New(t.Context(), store, catalog, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Enable(t.Context(), id)
	if err != nil || result.View.State != plugin.Enabled {
		t.Fatalf("committed result was reported as failed command: %+v, %v", result, err)
	}
	if !reflect.DeepEqual(result.Realization, Realization{Release: ReleaseAvailable}) {
		t.Fatalf("a one-shot reconciliation outcome replaced the observed realization: %+v", result.Realization)
	}
}

func TestListingPreservesCancellationInsteadOfProjectingUnavailability(t *testing.T) {
	store, dependencies, _, catalog := installationFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	cause := errors.New("listing abandoned")
	packages := installationPackages{realize: func(ctx context.Context, _ *plugin.Installation) (Realization, error) {
		cancel(cause)
		return Realization{}, context.Cause(ctx)
	}}
	c, err := New(t.Context(), store, catalog, packages, installationConnections{}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	inspections, err := c.List(ctx)
	if !errors.Is(err, cause) || len(inspections) != 0 {
		t.Fatalf("canceled listing = %+v, %v; want original cancellation cause", inspections, err)
	}
}

func TestListingReportsUnavailableReleaseWithoutLosingTheInstallation(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	packages := installationPackages{realize: func(context.Context, *plugin.Installation) (Realization, error) {
		return Realization{Release: ReleaseUnavailable}, nil
	}}
	c, err := New(t.Context(), store, catalog, packages, installationConnections{}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	inspections, err := c.List(t.Context())
	if err != nil || len(inspections) != 1 {
		t.Fatalf("unavailable listing = %+v, %v", inspections, err)
	}
	if inspections[0].View.ID != id || !reflect.DeepEqual(inspections[0].Realization, Realization{Release: ReleaseUnavailable}) {
		t.Fatalf("unavailable listing lost the installation or its realization: %+v", inspections[0])
	}
}

func TestPresentationIsAdmittedOnlyForAnActiveAvailableRelease(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	releaseLost := false
	packages := installationPackages{realize: func(context.Context, *plugin.Installation) (Realization, error) {
		if releaseLost {
			return Realization{Release: ReleaseUnavailable}, nil
		}
		return Realization{Release: ReleaseAvailable}, nil
	}}
	connections := installationConnections{reconcile: func(context.Context, []mcpserver.ID) error { return nil }}
	c, err := New(t.Context(), store, catalog, packages, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	presentation := func() Presentation {
		t.Helper()
		listed, err := c.List(t.Context())
		if err != nil || len(listed) != 1 {
			t.Fatalf("listing = %+v, %v", listed, err)
		}
		return listed[0].Presentation
	}
	if got := presentation(); got != PresentationWithheld {
		t.Fatalf("approved but disabled installation presentation = %q", got)
	}
	enabled, err := c.Enable(t.Context(), id)
	if err != nil || enabled.Presentation != PresentationAdmitted || presentation() != PresentationAdmitted {
		t.Fatalf("enabled installation presentation = %+v, %v", enabled, err)
	}
	releaseLost = true
	if got := presentation(); got != PresentationWithheld {
		t.Fatalf("enabled installation with unavailable release presentation = %q", got)
	}
	releaseLost = false
	if _, err := c.Disable(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if got := presentation(); got != PresentationWithheld {
		t.Fatalf("disabled installation presentation = %q", got)
	}
}

func TestStaleConfigurationNeverReachesPersistenceOrRealization(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	before, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(t.Context(), store, catalog, installationPackages{prepare: func(context.Context, *plugin.Installation) error {
		t.Fatal("stale configuration reached preparation")
		return nil
	}}, installationConnections{reconcile: func(context.Context, []mcpserver.ID) error {
		t.Fatal("stale configuration reached reconciliation")
		return nil
	}}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Configure(t.Context(), id, testsupport.Digest("2"), plugin.Configuration{Servers: map[mcpserver.ServerName]plugin.ComponentChange{testsupport.ServerName("server"): plugin.DisableComponent}}); !errors.Is(err, plugin.ErrStale) {
		t.Fatalf("stale configuration = %v", err)
	}
	after, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Installation.Snapshot(), after.Installation.Snapshot()) {
		t.Fatal("stale configuration advanced durable installation facts")
	}
}

func TestFailedPreparationIsObservedByTheChangeAndEveryLaterListing(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	backend, err := mcpserver.ParseServerName("server")
	if err != nil {
		t.Fatal(err)
	}
	unavailableBackend := Realization{Release: ReleaseAvailable, Sources: []mcpapp.Source{{
		Server:       mcpserver.Server{Name: backend},
		Availability: mcpapp.SourceUnavailableBackend,
	}}}
	reconciled := false
	packages := installationPackages{
		prepare: func(ctx context.Context, installation *plugin.Installation) error {
			if dependencies.held {
				t.Fatal("package preparation retained executable admission")
			}
			current, err := store.Get(ctx, id)
			if err != nil || !current.Installation.Active() || !installation.Active() {
				t.Fatal("package preparation preceded durable enablement")
			}
			return errors.New("backend directory could not be created")
		},
		realize: func(context.Context, *plugin.Installation) (Realization, error) {
			return unavailableBackend, nil
		},
	}
	connections := installationConnections{reconcile: func(context.Context, []mcpserver.ID) error {
		reconciled = true
		return nil
	}}
	c, err := New(t.Context(), store, catalog, packages, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Enable(t.Context(), id)
	if err != nil || result.View.State != plugin.Enabled || !reconciled || !slices.Equal(result.Realization.UnavailableBackends(), []mcpserver.ServerName{backend}) {
		t.Fatalf("committed change realization = %+v, %v", result, err)
	}
	for range 2 {
		listed, err := c.List(t.Context())
		if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0].Realization, unavailableBackend) {
			t.Fatalf("later listing lost the observed preparation failure: %+v, %v", listed, err)
		}
	}
}

func TestUninstallWithdrawsDurablyBeforeFailedCleanup(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ID) error {
		if dependencies.held {
			t.Fatal("cleanup retained executable admission")
		}
		if _, err := store.Get(ctx, id); !errors.Is(err, plugin.ErrNotFound) {
			t.Fatalf("uninstalled source can be restored: %v", err)
		}
		return errors.New("retirement failed")
	}}
	c, err := New(t.Context(), store, catalog, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Uninstall(t.Context(), id); err != nil {
		t.Fatalf("durable removal reported cleanup as a failed command: %v", err)
	}
}

// The live catalog is a projection of the committed installation: its
// superseded tools are withdrawn inside the admission critical section, after
// the durable write, so no Run admitted after the change can freeze them.
func TestChangedSourcesAreWithdrawnInsideAdmissionAfterTheCommit(t *testing.T) {
	for _, operation := range []string{"disable", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			store, dependencies, id, catalog := installationFixture(t)
			var withdrawn []mcpserver.ID
			changing := false
			connections := installationConnections{
				withdraw: func(names []mcpserver.ID) error {
					if !changing {
						return nil
					}
					if !dependencies.held {
						t.Fatal("withdrawal ran outside installation admission")
					}
					current, err := store.Get(t.Context(), id)
					switch operation {
					case "disable":
						if err != nil || current.Installation.Active() {
							t.Fatalf("withdrawal preceded the durable change: %+v, %v", current, err)
						}
					case "uninstall":
						if !errors.Is(err, plugin.ErrNotFound) {
							t.Fatalf("withdrawal preceded the durable removal: %v", err)
						}
					}
					withdrawn = append(withdrawn, names...)
					return nil
				},
				reconcile: func(_ context.Context, names []mcpserver.ID) error {
					if dependencies.held {
						t.Fatal("reconciliation retained installation admission")
					}
					if changing && !slices.Equal(names, withdrawn) {
						t.Fatalf("reconciled %v, withdrew %v", names, withdrawn)
					}
					return nil
				},
			}
			c, err := New(t.Context(), store, catalog, installationPackages{}, connections, dependencies, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Enable(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			changing = true
			switch operation {
			case "disable":
				_, err = c.Disable(t.Context(), id)
			case "uninstall":
				err = c.Uninstall(t.Context(), id)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(withdrawn) != 1 || withdrawn[0].Name().String() != "server" {
				t.Fatalf("withdrawn sources = %v", withdrawn)
			}
		})
	}
}

func TestInstallationReconcileCanReenterTheOwner(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	var c *Coordinator
	calls := 0
	connections := installationConnections{reconcile: func(ctx context.Context, _ []mcpserver.ID) error {
		calls++
		if calls == 1 {
			_, err := c.Disable(ctx, id)
			return err
		}
		return nil
	}}
	var err error
	c, err = New(t.Context(), store, catalog, installationPackages{}, connections, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Enable(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(t.Context(), id)
	if err != nil || current.Installation.Active() || calls != 2 {
		t.Fatalf("reentrant withdrawal: %+v, %v, calls=%d", current, err, calls)
	}
}

func TestCommittedInstallationRealizationFollowsRuntimeLifetime(t *testing.T) {
	for _, phase := range []string{"preparation", "reconciliation", "projection", "removal"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store, dependencies, id, catalog := installationFixture(t)
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
				connections := installationConnections{reconcile: func(context.Context, []mcpserver.ID) error { return nil }}
				switch phase {
				case "preparation":
					packages.prepare = func(ctx context.Context, _ *plugin.Installation) error { return wait(ctx) }
				case "reconciliation", "removal":
					connections.reconcile = func(ctx context.Context, _ []mcpserver.ID) error { return wait(ctx) }
				case "projection":
					packages.realize = func(ctx context.Context, _ *plugin.Installation) (Realization, error) {
						return Realization{}, wait(ctx)
					}
				}
				c, err := New(lifetime, store, catalog, packages, connections, dependencies, nil)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() {
					if phase == "removal" {
						done <- c.Uninstall(request, id)
						return
					}
					_, err := c.Enable(request, id)
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
					// A change result carries its observed realization. Once
					// shutdown preempts that observation the result reports the
					// runtime cause; the durable change itself stands (below).
					if phase == "removal" && err != nil {
						t.Fatalf("committed removal reported failure: %v", err)
					}
					if phase != "removal" && !errors.Is(err, cause) {
						t.Fatalf("unobservable realization = %v, want runtime cause", err)
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
				} else if store.record == nil || store.record.State != plugin.Enabled {
					t.Fatal("shutdown reversed durable enablement")
				}
			})
		})
	}
}

func TestInstallDecidesCapacityAtTheAdmissionSerializationPoint(t *testing.T) {
	dependencies := &installationDependencies{}
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package"})
	catalog := releaseMemory{release.Digest(): release}
	store := admittedInstallationStore{installationMemory: &installationMemory{catalog: catalog}, dependencies: dependencies}
	packages := installationPackages{materialize: func(context.Context, string) (plugin.Release, error) {
		if dependencies.held {
			t.Fatal("package materialization held installation admission")
		}
		return release, nil
	}}
	c, err := New(t.Context(), store, catalog, packages, installationConnections{}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := c.Install(t.Context(), "/package")
	if err != nil || installed.View.ID.Validate() != nil {
		t.Fatalf("install = %+v, %v", installed, err)
	}
}

func TestUninstallRequiresQuiescenceButRevocationDoesNot(t *testing.T) {
	store, dependencies, id, catalog := installationFixture(t)
	dependencies.inUse = true
	c, err := New(t.Context(), store, catalog, installationPackages{}, installationConnections{reconcile: func(context.Context, []mcpserver.ID) error { return nil }}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Uninstall(t.Context(), id); !errors.Is(err, plugin.ErrInUse) {
		t.Fatalf("uninstall of a depended installation = %v, want in-use refusal", err)
	}
	if _, err := store.Get(t.Context(), id); err != nil {
		t.Fatalf("refused uninstall removed the installation: %v", err)
	}
	if _, err := c.Revoke(t.Context(), id); err != nil {
		t.Fatalf("revocation of a depended installation: %v", err)
	}
}
