package plugin_test

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestToolCallerFencesOriginalInvocationAndInstalledAuthority(t *testing.T) {
	id := testsupport.InstallationID(t)
	server := testsupport.InstallationMCPServer(id, "schedules")
	value := plugin.ToolCallerSnapshot{Invocation: extension.Invocation{ID: "logical-tool", Parent: "process:original", Origin: extension.Model, Target: extension.Target{Scope: extension.ScopeInstallation, Reference: id.String()}}, Tool: testsupport.MCPTool(server, "run_schedule"), Release: testsupport.Digest("release"), Authority: testsupport.Digest("authority")}
	caller, err := plugin.NewToolCaller(value)
	if err != nil {
		t.Fatal(err)
	}
	if !caller.Matches(value.Invocation, id, value.Release, value.Authority, server.Name()) {
		t.Fatal("original source was refused")
	}
	for _, edit := range []func(*plugin.ToolCallerSnapshot){
		func(v *plugin.ToolCallerSnapshot) { v.Invocation.ID = "another-call" },
		func(v *plugin.ToolCallerSnapshot) { v.Invocation.Parent = "process:another" },
		func(v *plugin.ToolCallerSnapshot) { v.Release = testsupport.Digest("replacement") },
		func(v *plugin.ToolCallerSnapshot) { v.Authority = testsupport.Digest("changed-authority") },
	} {
		changed := value
		edit(&changed)
		if caller.Matches(changed.Invocation, id, changed.Release, changed.Authority, server.Name()) {
			t.Fatal("altered calling evidence acquired original authority")
		}
	}
	for _, edit := range []func(*plugin.ToolCallerSnapshot){
		func(v *plugin.ToolCallerSnapshot) { v.Invocation.Origin = extension.Human },
		func(v *plugin.ToolCallerSnapshot) { v.Invocation.Parent = "" },
		func(v *plugin.ToolCallerSnapshot) { v.Invocation.Target.Reference = "another-installation" },
		func(v *plugin.ToolCallerSnapshot) { v.Release = identity.Digest{} },
		func(v *plugin.ToolCallerSnapshot) {
			v.Tool = testsupport.MCPTool(testsupport.UserMCPServer("schedules"), "run_schedule")
		},
	} {
		invalid := value
		edit(&invalid)
		if _, err := plugin.NewToolCaller(invalid); err == nil {
			t.Fatal("invalid installed caller was admitted")
		}
	}
}
