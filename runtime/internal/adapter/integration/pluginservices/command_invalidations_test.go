package pluginservices

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
)

func TestCommandInvalidationsMatchConsumerContractInsteadOfReference(t *testing.T) {
	memory, err := memoryrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	schedules, err := schedulerpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	var notices []invalidation.Notice
	observe, err := CommandInvalidations(func(notice invalidation.Notice) { notices = append(notices, notice) })
	if err != nil {
		t.Fatal(err)
	}
	observe(plugin.CommandBinding{Contract: memory.Capability.Identity()})
	observe(plugin.CommandBinding{Contract: schedules.Capability.Identity()})
	if len(notices) != 2 || notices[0].Resource != invalidation.AgentMemory || notices[1].Resource != invalidation.Schedules {
		t.Fatalf("consumer notices: %+v", notices)
	}
	changed := memory.Capability.Descriptor()
	changed.Methods[0].Target = "workspace"
	other, err := capability.FromDescriptor(changed)
	if err != nil {
		t.Fatal(err)
	}
	observe(plugin.CommandBinding{Contract: other.Identity()})
	if len(notices) != 2 {
		t.Fatal("a same-named different contract invalidated this consumer")
	}
}

func TestBackgroundNoticesRequireTheirOwnEnabledContribution(t *testing.T) {
	name := testsupport.ServerName("schedules")
	contract, err := schedulerpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.background", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}, Contributions: []plugin.Contribution{{ID: "owner", Server: name, Export: "schedules", Contract: contract.Capability.Identity()}}})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	actor, err := operation.NewActor(installation.ID().String(), release.Digest().String(), testsupport.Digest("2").String(), name.String())
	if err != nil {
		t.Fatal(err)
	}
	var notices []invalidation.Notice
	caller := &CommandCalls{installations: store, publish: func(n invalidation.Notice) { notices = append(notices, n) }}
	if err := caller.NotifyChange(context.Background(), "owner"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("human forged a backend notice: %v", err)
	}
	ctx := operation.WithActor(t.Context(), actor)
	if err := caller.NotifyChange(ctx, "missing"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("undeclared contribution notice accepted: %v", err)
	}
	if err := caller.NotifyChange(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Resync {
		t.Fatalf("owner invalidation=%+v", notices)
	}
	installation.Disable()
	if err := caller.NotifyChange(ctx, "owner"); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("withdrawn owner notice accepted: %v", err)
	}
	if len(notices) != 1 {
		t.Fatal("refused source published a notice")
	}
}
