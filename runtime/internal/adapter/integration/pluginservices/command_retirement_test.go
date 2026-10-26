package pluginservices

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

type cancelPreparedStore struct {
	plugins.CommandStore
	cancel context.CancelFunc
}

func (s cancelPreparedStore) Save(ctx context.Context, command *plugin.Command) error {
	if err := s.CommandStore.Save(ctx, command); err != nil {
		return err
	}
	if command.Snapshot().Phase == plugin.CommandPrepared {
		s.cancel()
	}
	return nil
}

func TestCanceledPreparedStreamRetiresWithoutAnyBackendInvocation(t *testing.T) {
	f := newStreamFixture(t, "complete")
	ctx, cancel := context.WithCancel(operation.Bind(t.Context(), "records.stream", "cancel-after-binding"))
	defer cancel()
	store := f.host.PluginCommands
	routing, err := plugins.NewCommands(cancelPreparedStore{CommandStore: store, cancel: cancel}, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	f.calls.routing = routing
	if _, err := f.run(t, ctx); err == nil || f.invocations.Load() != 0 {
		t.Fatalf("canceled stream=%v, invocations=%d", err, f.invocations.Load())
	}
	id := operation.Identity(ctx)
	command, found, err := store.Get(t.Context(), id)
	if err != nil || !found || command.Snapshot().Phase != plugin.CommandPrepared {
		t.Fatalf("prepared=%v, %t, %v", command, found, err)
	}
	if err := f.host.Close(); err != nil {
		t.Fatal(err)
	}
	f.host, err = persistencefixture.Open(t, t.Context(), filepath.Dir(f.hostPath))
	if err != nil {
		t.Fatal(err)
	}
	store = f.host.PluginCommands
	f.calls.routing, err = plugins.NewCommands(store, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.calls.RetireCommand(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.calls.Command(t.Context(), id); !errors.Is(err, plugin.ErrCommandRetired) {
		t.Fatalf("recovery=%v", err)
	}
	if _, err := f.run(t, operation.Bind(t.Context(), "records.stream", "cancel-after-binding")); !errors.Is(err, plugin.ErrCommandRetired) || f.invocations.Load() != 0 {
		t.Fatalf("retired stream replayed: %v, invocations=%d", err, f.invocations.Load())
	}
	if held, err := store.PendingCommandDependencies(t.Context()); err != nil || len(held) != 0 {
		t.Fatalf("retired dependencies=%v, %v", held, err)
	}
}
