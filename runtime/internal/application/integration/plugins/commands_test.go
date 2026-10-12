package plugins_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestCommandsKeepOriginalRouteAndAdmitDispatchOnlyOnce(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewPluginCommandStore(db)
	commands, err := plugins.NewCommands(store, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	provider := plugins.ServiceBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend")}
	resolve := func(context.Context) (plugins.ServiceBinding, error) { return provider, nil }
	bound, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), resolve)
	if err != nil {
		t.Fatal(err)
	}
	if err := commands.Observe(t.Context(), "command"); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("observation before dispatch = %v", err)
	}
	provider.Digest = testsupport.Digest("3")
	replay, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.ServiceBinding, error) {
		t.Fatal("same identity resolved a current provider")
		return provider, nil
	})
	if err != nil || replay.Snapshot() != bound.Snapshot() {
		t.Fatalf("original binding = %v, %v", replay, err)
	}
	if _, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":4}`), resolve); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("changed input = %v", err)
	}
	refused := errors.New("original provider unavailable")
	if err := commands.Dispatch(t.Context(), "command", func(context.Context, plugin.CommandBinding) error { return refused }); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	prepared, _, err := store.Get(t.Context(), "command")
	if err != nil || prepared.Snapshot().Phase != plugin.CommandPrepared {
		t.Fatalf("rejected admission advanced evidence = %v, %v", prepared, err)
	}
	if err := commands.Dispatch(t.Context(), "command", func(_ context.Context, route plugin.CommandBinding) error {
		if route != bound.Snapshot() {
			t.Fatal("dispatch changed the route")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := commands.Dispatch(t.Context(), "command", func(context.Context, plugin.CommandBinding) error {
		t.Fatal("already admitted command was dispatched again")
		return nil
	}); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatalf("duplicate dispatch = %v", err)
	}
	if err := commands.Observe(t.Context(), "command"); err != nil {
		t.Fatal(err)
	}
	held, err := store.PendingCommandDependencies(t.Context())
	if err != nil || len(held) != 0 {
		t.Fatalf("observed dependency = %v, %v", held, err)
	}
	retained, _, err := store.Get(t.Context(), "command")
	if err != nil || retained.Snapshot().Provider != bound.Snapshot().Provider {
		t.Fatalf("original route lost = %v, %v", retained, err)
	}
}

type blockedCommandStore struct {
	persist <-chan struct{}
	command *plugin.Command
}

func (s *blockedCommandStore) Get(context.Context, string) (*plugin.Command, bool, error) {
	return s.command, s.command != nil, nil
}
func (s *blockedCommandStore) Save(_ context.Context, command *plugin.Command) error {
	<-s.persist
	s.command = command
	return nil
}

func TestCommandBindingAndInstallationChangeShareAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := plugins.NewAdmission()
		persist := make(chan struct{})
		store := &blockedCommandStore{persist: persist}
		commands, err := plugins.NewCommands(store, admission)
		if err != nil {
			t.Fatal(err)
		}
		provider := plugins.ServiceBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend")}
		bound := make(chan error, 1)
		go func() {
			_, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.ServiceBinding, error) { return provider, nil })
			bound <- err
		}()
		synctest.Wait()
		changed := make(chan error, 1)
		go func() {
			changed <- admission.Change(t.Context(), provider.InstallationID, func() (bool, error) {
				if store.command == nil || !store.command.Pending() {
					return false, errors.New("change missed the pending command")
				}
				return false, plugin.ErrInUse
			})
		}()
		synctest.Wait()
		select {
		case err := <-changed:
			t.Fatalf("change escaped pending persistence: %v", err)
		default:
		}
		close(persist)
		synctest.Wait()
		if err := <-bound; err != nil {
			t.Fatal(err)
		}
		if err := <-changed; !errors.Is(err, plugin.ErrInUse) {
			t.Fatalf("change missed committed dependency: %v", err)
		}
	})
}
