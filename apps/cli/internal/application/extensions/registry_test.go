package extensions

import (
	"errors"
	"math"
	"testing"
)

func TestPluginClaimSequenceExhaustionPreservesLoadedOwners(t *testing.T) {
	registry := &commandRegistry{next: registrationSequence(math.MaxUint64), plugins: map[string]registrationSequence{"existing": 1}}
	if _, err := registry.claim("replacement"); !errors.Is(err, errRegistrationSequenceExhausted) {
		t.Fatalf("claim after exhaustion = %v", err)
	}
	if len(registry.plugins) != 1 || registry.plugins["existing"] != 1 {
		t.Fatalf("plugin owners = %+v", registry.plugins)
	}
}
func TestCommandSequenceExhaustionLeavesRegistryUnchanged(t *testing.T) {
	registry := &commandRegistry{next: registrationSequence(math.MaxUint64)}
	if _, err := registry.insert("plugin", testCommand("hello")); !errors.Is(err, errRegistrationSequenceExhausted) {
		t.Fatalf("registration after exhaustion = %v", err)
	}
	if len(registry.entries) != 0 {
		t.Fatalf("commands after exhaustion = %+v", registry.entries)
	}
}
func TestCommandNamespaceRejectsNamesAndAliasesWithoutConsumingSequence(t *testing.T) {
	registry := new(commandRegistry)
	command := testCommand("hello")
	command.Descriptor.Aliases = []string{"greet"}
	if _, err := registry.insert("first", command); err != nil {
		t.Fatal(err)
	}
	before := registry.next
	for _, name := range []string{"hello", "greet"} {
		if _, err := registry.insert("second", testCommand(name)); err == nil {
			t.Fatalf("accepted duplicate identity %q", name)
		}
	}
	if registry.next != before {
		t.Fatalf("rejected registration advanced sequence: %d", registry.next)
	}
}
func TestCommandsRetainProviderOrderAndDetachedDescriptors(t *testing.T) {
	host := NewHost()
	command := testCommand("first")
	command.Descriptor.Aliases = []string{"alias"}
	results, err := host.Activate([]Plugin{manifest("test.ordered", func(scope *Scope) error {
		if err := scope.ContributeCommand(command); err != nil {
			return err
		}
		return scope.ContributeCommand(testCommand("second"))
	})})
	if err != nil || !allLoaded(results) {
		t.Fatalf("activation = %+v, %v", results, err)
	}
	command.Descriptor.Aliases[0] = "caller-change"
	values := host.Commands()
	if len(values) != 2 || values[0].PluginID != "test.ordered" || values[0].Command.Descriptor.Name != "first" || values[1].Command.Descriptor.Name != "second" || values[0].Command.Descriptor.Aliases[0] != "alias" {
		t.Fatalf("commands = %+v", values)
	}
	values[0].Command.Descriptor.Aliases[0] = "reader-change"
	if host.Commands()[0].Command.Descriptor.Aliases[0] != "alias" {
		t.Fatal("reader changed retained descriptor")
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	if len(host.Commands()) != 0 {
		t.Fatal("close retained command registrations")
	}
}
func TestSetupFailureRollsBackCommandsAndPluginClaim(t *testing.T) {
	registry := new(commandRegistry)
	want := errors.New("setup failed")
	_, err := load(registry, manifest("test.failed", func(scope *Scope) error {
		if err := scope.ContributeCommand(testCommand("hello")); err != nil {
			return err
		}
		return want
	}))
	if !errors.Is(err, want) || len(registry.commands()) != 0 {
		t.Fatalf("failed setup = %v, commands = %+v", err, registry.commands())
	}
	loaded, err := load(registry, manifest("test.failed", func(scope *Scope) error { return scope.ContributeCommand(testCommand("hello")) }))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.dispose(); err != nil {
		t.Fatal(err)
	}
}
func TestCompletedScopeRejectsLateCommands(t *testing.T) {
	registry := new(commandRegistry)
	var retained *Scope
	loaded, err := load(registry, manifest("test.complete", func(scope *Scope) error { retained = scope; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := retained.ContributeCommand(testCommand("late")); !errors.Is(err, errScopeClosed) {
		t.Fatalf("late command = %v", err)
	}
	if err := loaded.dispose(); err != nil {
		t.Fatal(err)
	}
}
func TestRegistrationRemovalCannotDeleteAReplacementOwner(t *testing.T) {
	registry := new(commandRegistry)
	first, err := registry.insert("first", testCommand("hello"))
	if err != nil {
		t.Fatal(err)
	}
	registry.remove("hello", first)
	second, err := registry.insert("second", testCommand("hello"))
	if err != nil {
		t.Fatal(err)
	}
	registry.remove("hello", first)
	values := registry.commands()
	if len(values) != 1 || values[0].PluginID != "second" {
		t.Fatalf("stale disposal changed replacement = %+v", values)
	}
	registry.remove("hello", second)
}
