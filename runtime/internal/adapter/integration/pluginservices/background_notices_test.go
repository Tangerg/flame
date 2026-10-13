package pluginservices

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestBackgroundNoticesRequireTheirOwnEnabledContribution(t *testing.T) {
	name := testsupport.ServerName("records")
	contract, err := capability.New(capability.Config{Reference: "records", Scope: "installation", Composition: capability.Selection}, capability.MethodDescriptor{Name: "records.read", Kind: capability.Query, Target: "installation", Description: "Read records.", Input: []byte(`{"type":"object","additionalProperties":false}`), Output: []byte(`{"type":"string"}`)})
	if err != nil {
		t.Fatal(err)
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.background", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}, Contributions: []plugin.Contribution{{ID: "owner", Server: name, Export: "records", Contract: contract.Identity()}}})
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
	if len(notices) != 1 || notices[0].Resource != invalidation.PluginData {
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
