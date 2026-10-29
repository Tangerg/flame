package persistence

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type bindingPlanningDriver struct{}

func (bindingPlanningDriver) Profile() qimu.Profile { return qimu.MCPProcess }
func (bindingPlanningDriver) Prepare(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
	return nil, errors.New("durable binding fixture does not execute a backend")
}

type bindingFixture struct {
	store      *Bundle
	host       *qimu.Host
	definition *qimu.Plugin
	activation plugin.Activation
	provider   plugin.Dependency
}

func newBindingFixture(t *testing.T) bindingFixture {
	t.Helper()
	store, err := Open(t.Context(), Config{DataDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	host, err := qimu.NewHost(context.Background(), qimu.HostConfig{
		Admission: store.Admission, Epoch: store.Qimu.Epoch(),
		Installations: store.Qimu, ExecutionProtection: store.Qimu,
		Drivers: []qimu.Driver{bindingPlanningDriver{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	release := testsupport.Release(t, "retained", plugin.Declaration{Name: "retained", Servers: []plugin.Server{{
		Name: testsupport.ServerName("backend"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp",
	}}})
	if err := store.PluginReleases.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	projection, err := plugins.ProjectFrameworkRelease(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.RegisterFrameworkRelease(host, projection); err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "fixture", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	id := qimu.InstallationID(installation.ID().String())
	if _, err := host.Install(t.Context(), qimu.InstallationConfig{ID: id, Plugin: projection.Plugin, Configuration: configuration, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan, err := host.PlanUnit(t.Context(), "", qimu.UnitRef{InstallationID: id, Release: projection.Plugin.Revision(), UnitID: plugins.BackendUnit(testsupport.ServerName("backend"))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var binding qimu.BindingSnapshot
	if err := host.Admission().Admit(t.Context(), func(admitted context.Context) error {
		var err error
		binding, err = host.ValidatePlanUnder(admitted, plan)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	activation, err := plugins.ProjectActivation(binding)
	if err != nil {
		t.Fatal(err)
	}
	return bindingFixture{store, host, projection.Plugin, activation, plugin.Dependency{InstallationID: installation.ID(), Digest: release.Digest()}}
}

func (f bindingFixture) checkpoint() runs.ExecutorCheckpoint {
	value := storedExecutorCheckpoint("member_retained", "ses_retained", "original continuation")
	value.Bindings = []plugin.Activation{f.activation.Clone()}
	value.Resources = []plugin.Dependency{f.provider}
	return value
}

func (f bindingFixture) saveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	_, err := sqlite.Connection(ctx, f.store.db).ExecContext(ctx, `INSERT INTO runs(run_id,session_id,state,created_at,updated_at) VALUES('run_retained',?,'waiting',0,0) ON CONFLICT(run_id) DO NOTHING`, checkpoint.SessionID)
	if err != nil {
		return err
	}
	return f.store.ExecutorCheckpoints.SaveCheckpoint(ctx, checkpoint)
}

func TestPendingPluginBindingsReadsOriginalOwnersAfterColdReopen(t *testing.T) {
	f := newBindingFixture(t)
	checkpoint := f.checkpoint()
	if err := f.store.CommitPluginReferences(t.Context(), f.host, "original-checkpoint", checkpoint, func(ctx context.Context) error {
		return f.saveCheckpoint(ctx, checkpoint)
	}); err != nil {
		t.Fatal(err)
	}
	bindings, err := f.store.PendingPluginBindings(t.Context())
	if err != nil || len(bindings) != 1 || !bindings[0].Equal(f.activation) {
		t.Fatalf("original pending binding = %+v, %v", bindings, err)
	}
	bindings[0].Dependencies = nil
	if _, err := f.host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), Config{DataDirectory: f.store.DataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	bindings, err = reopened.PendingPluginBindings(t.Context())
	if err != nil || len(bindings) != 1 || !bindings[0].Equal(f.activation) {
		t.Fatalf("cold pending binding was reselected or mutated: %+v, %v", bindings, err)
	}
	if err := reopened.ExecutorCheckpoints.DeleteCheckpoints(t.Context(), checkpoint.SessionID, []string{checkpoint.RootMemberID}); err != nil {
		t.Fatal(err)
	}
	bindings, err = reopened.PendingPluginBindings(t.Context())
	if err != nil || len(bindings) != 0 {
		t.Fatalf("removed original owner survived in a copied registry: %+v, %v", bindings, err)
	}
}

func TestBusinessCheckpointAdmitsOriginalBindingInItsOwnAtomicWriteSet(t *testing.T) {
	f := newBindingFixture(t)
	checkpoint := f.checkpoint()
	if err := f.store.ExecutorCheckpoints.SaveCheckpoint(t.Context(), checkpoint); err == nil {
		t.Fatal("a direct projection write created durable plugin references")
	}
	failed := errors.New("original business write rejected")
	err := f.store.CommitPluginReferences(t.Context(), f.host, "failed-event", checkpoint, func(ctx context.Context) error {
		if err := f.saveCheckpoint(ctx, checkpoint); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("failed transaction = %v", err)
	}
	if _, err := f.store.ExecutorCheckpoints.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("rolled-back original binding remained visible: %v", err)
	}
	if held, err := f.store.PluginUses.Held(t.Context(), qimu.InstallationID(f.provider.InstallationID.String())); err != nil || len(held) != 0 {
		t.Fatalf("rollback retained references: %+v, %v", held, err)
	}
	err = f.store.CommitPluginReferences(t.Context(), f.host, "original-event", checkpoint, func(ctx context.Context) error {
		return f.saveCheckpoint(ctx, checkpoint)
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := f.store.ExecutorCheckpoints.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil || !reflect.DeepEqual(loaded, checkpoint) {
		t.Fatalf("original checkpoint = %+v, %v", loaded, err)
	}
	if err := f.host.Uninstall(t.Context(), qimu.InstallationID(f.provider.InstallationID.String())); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("installation escaped retained business graph: %v", err)
	}
	if err := f.host.SetEnabled(t.Context(), qimu.InstallationID(f.provider.InstallationID.String()), false); err != nil {
		t.Fatal(err)
	}
	checkpoint.Payload = []byte("original execution progressed while disabled")
	err = f.store.CommitPluginReferences(t.Context(), f.host, "progress-event", checkpoint, func(ctx context.Context) error {
		return f.saveCheckpoint(ctx, checkpoint)
	})
	if err != nil {
		t.Fatalf("an original retained graph could not finish: %v", err)
	}
	if pending, err := f.store.PluginUses.Pending(t.Context()); err != nil || len(pending) != 0 {
		t.Fatalf("stable storage evidence leaked technical submissions: %+v, %v", pending, err)
	}
	if err := f.store.ExecutorCheckpoints.DeleteSessionCheckpoints(t.Context(), checkpoint.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Uninstall(t.Context(), qimu.InstallationID(f.provider.InstallationID.String())); err != nil {
		t.Fatalf("business retirement did not release the original source: %v", err)
	}
}

func TestBusinessCheckpointAndInstallationChangeHaveOneAdmission(t *testing.T) {
	for _, first := range []string{"checkpoint", "uninstall"} {
		t.Run(first, func(t *testing.T) {
			f := newBindingFixture(t)
			checkpoint := f.checkpoint()
			id := qimu.InstallationID(f.provider.InstallationID.String())
			if first == "uninstall" {
				if err := f.host.Uninstall(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				err := f.store.CommitPluginReferences(t.Context(), f.host, "late-checkpoint", checkpoint, func(context.Context) error {
					t.Fatal("a stale original graph reached its business write-set")
					return nil
				})
				if !errors.Is(err, qimu.ErrStaleBinding) && !errors.Is(err, qimu.ErrUnavailable) {
					t.Fatalf("removed binding admitted = %v", err)
				}
				return
			}
			entered, release := make(chan struct{}), make(chan struct{})
			committed, changed := make(chan error, 1), make(chan error, 1)
			go func() {
				committed <- f.store.CommitPluginReferences(t.Context(), f.host, "concurrent-checkpoint", checkpoint, func(ctx context.Context) error {
					if err := f.saveCheckpoint(ctx, checkpoint); err != nil {
						return err
					}
					close(entered)
					<-release
					return nil
				})
			}()
			<-entered
			go func() { changed <- f.host.Uninstall(t.Context(), id) }()
			close(release)
			if err := <-committed; err != nil {
				t.Fatal(err)
			}
			if err := <-changed; !errors.Is(err, qimu.ErrInUse) {
				t.Fatalf("installation missed the committed original reference: %v", err)
			}
		})
	}
}

func TestOriginalCommandRetainsBindingUntilItsBusinessOwnerRetires(t *testing.T) {
	f := newBindingFixture(t)
	commands, err := plugins.NewCommands(f.store.PluginCommands, f.store.Admission)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := plugins.DecodeActivation(f.activation)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(ctx context.Context) (plugins.CommandAdmission, error) {
		if err := f.host.ValidateBindingUnder(ctx, binding); err != nil {
			return plugins.CommandAdmission{}, err
		}
		return plugins.CommandAdmission{Activation: f.activation, Binding: plugins.ContributionBinding{
			InstallationID: f.provider.InstallationID, Digest: f.provider.Digest,
			ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "records.write", extension.ScopeInstallation), ContributionID: "records", Export: "records",
		}}, nil
	}
	command, err := commands.Bind(t.Context(), "original-command", "records.write", testsupport.JSONInput(t, `{}`), resolve, func(plugins.ContributionBinding) (portable.Invocation, error) {
		return portable.Invocation{ID: "original-command", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: f.provider.InstallationID.String()}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id := qimu.InstallationID(f.provider.InstallationID.String())
	if err := f.host.Uninstall(t.Context(), id); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("prepared command did not protect its original graph: %v", err)
	}
	if err := commands.Retire(t.Context(), command.Snapshot().ID); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Uninstall(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	retained, found, err := f.store.PluginCommands.Get(t.Context(), "original-command")
	if err != nil || !found || !retained.Snapshot().Activation.Equal(f.activation) || retained.Snapshot().Phase != plugin.CommandRetired {
		t.Fatalf("business retirement erased the original route: %+v, %v", retained, err)
	}
}

func TestCheckpointCommitRefusesChangesToItsAdmittedWriteSet(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*runs.ExecutorCheckpoint)
	}{
		{name: "payload", mutate: func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Payload = []byte("different continuation") }},
		{name: "build", mutate: func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.BuildID = testsupport.AlternateBuildID }},
		{name: "references removed", mutate: func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Bindings = nil; checkpoint.Resources = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newBindingFixture(t)
			admitted := f.checkpoint()
			replacement := admitted.Clone()
			test.mutate(&replacement)
			err := f.store.CommitPluginReferences(t.Context(), f.host, "altered-checkpoint", admitted, func(ctx context.Context) error {
				return f.saveCheckpoint(ctx, replacement)
			})
			if err == nil {
				t.Fatal("a changed checkpoint committed under the original write-set identity")
			}
			if _, err := f.store.ExecutorCheckpoints.LoadCheckpoint(t.Context(), admitted.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
				t.Fatalf("rejected checkpoint became durable: %v", err)
			}
			if err := f.store.CommitPluginReferences(t.Context(), f.host, "exact-checkpoint", admitted, func(ctx context.Context) error {
				return f.saveCheckpoint(ctx, admitted)
			}); err != nil {
				t.Fatalf("original checkpoint refused: %v", err)
			}
		})
	}
}
