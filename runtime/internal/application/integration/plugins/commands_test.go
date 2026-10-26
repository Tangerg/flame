package plugins_test

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestCommandsKeepOriginalRouteAndAdmitDispatchOnlyOnce(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := db.PluginCommands
	commands, err := plugins.NewCommands(store, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", "installation"), ContributionID: "original-provider", Export: "operations"}
	resolve := func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }
	bound, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), resolve, capturedCommand)
	if err != nil {
		t.Fatal(err)
	}
	if err := commands.Observe(t.Context(), "command"); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("observation before dispatch = %v", err)
	}
	if _, err := commands.Get(t.Context(), "command"); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatalf("recovery exposed a route without dispatch admission: %v", err)
	}
	if _, err := commands.Get(t.Context(), "missing"); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("missing route was invented: %v", err)
	}
	provider.Digest = testsupport.Digest("3")
	replay, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.CommandAdmission, error) {
		t.Fatal("same identity resolved a current provider")
		return commandAdmission(provider), nil
	}, capturedCommand)
	if err != nil || !reflect.DeepEqual(replay.Snapshot(), bound.Snapshot()) {
		t.Fatalf("original binding = %v, %v", replay, err)
	}
	if _, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":4}`), resolve, capturedCommand); !errors.Is(err, plugin.ErrCommandConflict) {
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
		if !reflect.DeepEqual(route, bound.Snapshot()) {
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
	original, err := commands.Get(t.Context(), "command")
	if err != nil || original.Snapshot().Input != bound.Snapshot().Input || original.Snapshot().Provider != bound.Snapshot().Provider {
		t.Fatalf("recovery lost original input or provider: %v, %v", original, err)
	}
	actor, err := operation.NewActor(testsupport.InstallationID(t).String(), testsupport.Digest("actor").String(), testsupport.Digest("authority").String(), "backend")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.Get(operation.WithActor(t.Context(), actor), "command"); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("an undeclared managed reader obtained another command's input: %v", err)
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
		provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", "installation"), ContributionID: "original-provider", Export: "operations"}
		bound := make(chan error, 1)
		go func() {
			_, err := commands.Bind(t.Context(), "command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }, capturedCommand)
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

func capturedCommand(binding plugins.ContributionBinding) (capability.Invocation, error) {
	invocation := capability.Invocation{ID: "command", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: binding.InstallationID.String()}}
	return invocation, invocation.Validate()
}

func TestBackgroundRestoresOnlyItsDispatchedOriginalCaller(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	commands, err := plugins.NewCommands(db.PluginCommands, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", "installation"), ContributionID: "owner", Export: "operations"}
	original, err := capturedCommand(provider)
	if err != nil {
		t.Fatal(err)
	}
	original.Parent = "original-parent"
	command, err := commands.Bind(t.Context(), original.ID, "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }, func(plugins.ContributionBinding) (capability.Invocation, error) { return original, nil })
	if err != nil {
		t.Fatal(err)
	}
	actor, err := operation.NewActor(provider.InstallationID.String(), provider.Digest.String(), testsupport.Digest("2").String(), provider.ServerName.String())
	if err != nil {
		t.Fatal(err)
	}
	calling := operation.WithActor(t.Context(), actor)
	if _, err := commands.RestoreInvocation(calling, original); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("unsent original was accepted: %v", err)
	}
	if err := commands.Dispatch(t.Context(), command.Snapshot().ID, func(context.Context, plugin.CommandBinding) error { return nil }); err != nil {
		t.Fatal(err)
	}
	restored, err := commands.RestoreInvocation(calling, original)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := operation.Capture(restored, original.Target)
	if err != nil || caller != original || operation.Identity(restored) != original.ID {
		t.Fatalf("caller was replaced: %+v, %v", caller, err)
	}
	retained, found := operation.ActorFrom(restored)
	if !found || retained.InstallationID() != actor.InstallationID() || retained.Authority() != actor.Authority() {
		t.Fatal("invocation replaced authenticated Actor")
	}
	for _, change := range []func(*capability.Invocation){
		func(v *capability.Invocation) { v.Origin = capability.Model },
		func(v *capability.Invocation) { v.Parent = "changed-parent" },
		func(v *capability.Invocation) { v.ID = "unknown-human" },
		func(v *capability.Invocation) { v.Target.Reference = testsupport.InstallationID(t).String() },
	} {
		altered := original
		change(&altered)
		if _, err := commands.RestoreInvocation(calling, altered); !errors.Is(err, plugin.ErrCommandConflict) {
			t.Fatalf("changed caller accepted: %+v, %v", altered, err)
		}
	}
	automatic := capability.Invocation{ID: "private-trigger", Origin: capability.Automatic, Target: original.Target}
	if _, err := commands.RestoreInvocation(calling, automatic); err != nil {
		t.Fatal(err)
	}
	automatic.Parent = "unrecorded-parent"
	if _, err := commands.RestoreInvocation(calling, automatic); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("unrecorded parent accepted: %v", err)
	}
}

func TestBackgroundRestoresOriginalResourceTargetAfterRestart(t *testing.T) {
	for _, target := range []capability.Target{{Scope: "session", Reference: "ses_original"}, {Scope: "workspace", Reference: "/original/workspace"}} {
		for _, origin := range []capability.Origin{capability.Human, capability.Model, capability.Automatic} {
			t.Run(target.Scope+"/"+string(origin), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "flame.db")
				db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(path))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				commands, err := plugins.NewCommands(db.PluginCommands, plugins.NewAdmission())
				if err != nil {
					t.Fatal(err)
				}
				provider := plugins.ContributionBinding{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1"), ServerName: testsupport.ServerName("backend"), Contract: testsupport.CommandContract(t, "review.update", target.Scope), ContributionID: "owner", Export: "operations"}
				original := capability.Invocation{ID: "original-command", Origin: origin, Parent: "original-parent", Target: target}
				command, err := commands.Bind(t.Context(), original.ID, "review.update", testsupport.JSONInput(t, `{"revision":2}`), func(context.Context) (plugins.CommandAdmission, error) { return commandAdmission(provider), nil }, func(plugins.ContributionBinding) (capability.Invocation, error) { return original, nil })
				if err != nil {
					t.Fatal(err)
				}
				actor, err := operation.NewActor(provider.InstallationID.String(), provider.Digest.String(), testsupport.Digest("2").String(), provider.ServerName.String())
				if err != nil {
					t.Fatal(err)
				}
				calling := operation.WithActor(t.Context(), actor)
				if _, err := commands.RestoreInvocation(calling, original); !errors.Is(err, plugin.ErrCommandConflict) {
					t.Fatalf("prepared resource command restored: %v", err)
				}
				if err := commands.Dispatch(t.Context(), command.Snapshot().ID, func(context.Context, plugin.CommandBinding) error { return nil }); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				db, err = persistencefixture.Open(t, t.Context(), filepath.Dir(path))
				if err != nil {
					t.Fatal(err)
				}
				commands, err = plugins.NewCommands(db.PluginCommands, plugins.NewAdmission())
				if err != nil {
					t.Fatal(err)
				}
				for _, observe := range []bool{false, true} {
					if observe {
						if err := commands.Observe(t.Context(), original.ID); err != nil {
							t.Fatal(err)
						}
					}
					restored, err := commands.RestoreInvocation(calling, original)
					if err != nil {
						t.Fatal(err)
					}
					captured, err := operation.Capture(restored, target)
					retained, found := operation.ActorFrom(restored)
					if err != nil || captured != original || !found || retained != actor {
						t.Fatalf("original resource caller changed: %+v %v", captured, err)
					}
				}
				for _, change := range []func(*capability.Invocation){
					func(v *capability.Invocation) { v.Target.Reference = "different-resource" },
					func(v *capability.Invocation) {
						v.Target = capability.Target{Scope: "installation", Reference: actor.InstallationID().String()}
					},
					func(v *capability.Invocation) { v.Parent = "different-parent" },
				} {
					altered := original
					change(&altered)
					if _, err := commands.RestoreInvocation(calling, altered); !errors.Is(err, plugin.ErrCommandConflict) {
						t.Fatalf("resource caller retargeted: %+v %v", altered, err)
					}
				}
				fresh := capability.Invocation{ID: "unrecorded-resource-trigger", Origin: capability.Automatic, Target: target}
				if _, err := commands.RestoreInvocation(calling, fresh); !errors.Is(err, plugin.ErrCommandConflict) {
					t.Fatalf("unrecorded resource target accepted: %v", err)
				}
			})
		}
	}
}

func commandAdmission(binding plugins.ContributionBinding) plugins.CommandAdmission {
	return plugins.CommandAdmission{Binding: binding, Activation: plugin.Activation{Identity: testsupport.Digest("activation"), Dependencies: []plugin.Dependency{{InstallationID: binding.InstallationID, Digest: binding.Digest}}}}
}
