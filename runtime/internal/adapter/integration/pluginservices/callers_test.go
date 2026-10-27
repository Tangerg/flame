package pluginservices

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type emptyCallerCommands struct{}

func (emptyCallerCommands) Get(context.Context, string) (*plugin.Command, bool, error) {
	return nil, false, nil
}
func (emptyCallerCommands) Save(context.Context, *plugin.Command) error {
	return errors.New("unexpected command write")
}

type retainedToolCaller struct {
	value plugin.ToolCaller
	err   error
}

func (r retainedToolCaller) ToolCaller(_ context.Context, id string) (plugin.ToolCaller, bool, error) {
	return r.value, id == r.value.Snapshot().Invocation.ID, r.err
}

func TestInstalledCallerRestorationCannotBorrowAnotherActorOrInvocation(t *testing.T) {
	commands, err := plugins.NewCommands(emptyCallerCommands{}, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	id := testsupport.InstallationID(t)
	server := testsupport.InstallationMCPServer(id, "schedules")
	value := plugin.ToolCallerSnapshot{Invocation: extension.Invocation{ID: "original-model-call", Parent: "process:original", Origin: extension.Model, Target: extension.Target{Scope: extension.ScopeInstallation, Reference: id.String()}}, Tool: testsupport.MCPTool(server, "run_schedule"), Release: testsupport.Digest("release"), Authority: testsupport.Digest("authority")}
	caller, err := plugin.NewToolCaller(value)
	if err != nil {
		t.Fatal(err)
	}
	restorer, err := NewCallerRestorer(commands, retainedToolCaller{value: caller})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := operation.NewActor(id.String(), value.Release.String(), value.Authority.String(), server.Name().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx := operation.WithActor(t.Context(), actor)
	restored, err := restorer.RestoreInvocation(ctx, value.Invocation)
	if err != nil || operation.Identity(restored) != value.Invocation.ID {
		t.Fatalf("original calling provenance was not restored: %v", err)
	}
	if _, err := restorer.RestoreInvocation(t.Context(), value.Invocation); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("missing installed Actor gained authority: %v", err)
	}
	for _, edit := range []func(*extension.Invocation){
		func(i *extension.Invocation) { i.ID = "unrecorded-model-call" },
		func(i *extension.Invocation) { i.Parent = "process:other" },
		func(i *extension.Invocation) { i.Origin = extension.Human },
		func(i *extension.Invocation) { i.Target.Reference = testsupport.InstallationID(t).String() },
	} {
		changed := value.Invocation
		edit(&changed)
		if _, err := restorer.RestoreInvocation(ctx, changed); !errors.Is(err, plugin.ErrCommandConflict) {
			t.Fatalf("changed caller was restored: %+v, %v", changed, err)
		}
	}
	for _, values := range [][4]string{
		{testsupport.InstallationID(t).String(), value.Release.String(), value.Authority.String(), "schedules"},
		{id.String(), testsupport.Digest("replacement").String(), value.Authority.String(), "schedules"},
		{id.String(), value.Release.String(), testsupport.Digest("replacement-authority").String(), "schedules"},
		{id.String(), value.Release.String(), value.Authority.String(), "another-server"},
	} {
		changed, err := operation.NewActor(values[0], values[1], values[2], values[3])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := restorer.RestoreInvocation(operation.WithActor(t.Context(), changed), value.Invocation); !errors.Is(err, plugin.ErrCommandConflict) {
			t.Fatalf("another Actor gained original authority: %v", err)
		}
	}
	broken := errors.New("caller store unavailable")
	restorer, err = NewCallerRestorer(commands, retainedToolCaller{value: caller, err: broken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restorer.RestoreInvocation(ctx, value.Invocation); !errors.Is(err, broken) {
		t.Fatalf("routing failure was hidden: %v", err)
	}
}
