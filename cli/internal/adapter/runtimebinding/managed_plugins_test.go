package runtimebinding

import (
	json "encoding/json/v2"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

func activateManagedPlugins(t *testing.T, connection *Connection, features ...string) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "packages")
	builder, err := filepath.Abs("../../../../plugins/build.mjs")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "node", builder, "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", output)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build local plugin applications: %s %v", out, err)
	}
	for _, feature := range features {
		installed, err := connection.plugins.InstallPlugin(t.Context(), protocol.InstallPluginRequest{Source: filepath.Join(output, feature)}, flameruntime.CommandOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = connection.plugins.ApprovePlugin(t.Context(), protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, flameruntime.CommandOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = connection.plugins.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, flameruntime.CommandOptions{})
		if err != nil {
			t.Fatal(err)
		}
		server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: feature}
		deadline := time.Now().Add(10 * time.Second)
		for {
			tools, err := connection.mcp.ListMCPTools(t.Context(), protocol.MCPListToolsRequest{Server: &server}, connection.callOptions())
			if err != nil {
				t.Fatalf("read %s backend tools: %v", feature, err)
			}
			expected := 1
			if feature == "schedules" {
				expected = 3
			}
			if tools != nil && len(tools.Data) == expected {
				break
			}
			servers, err := connection.mcp.ListMCPServers(t.Context(), connection.callOptions())
			if err != nil {
				t.Fatalf("read %s backend status: %v", feature, err)
			}
			if servers != nil {
				for _, entry := range servers.Data {
					if entry.ID.Origin.InstallationID == installed.ID && entry.ID.Name == feature &&
						(entry.Status.Type == protocol.MCPServerFailed || entry.Status.Type == protocol.MCPServerNeedsAuth) {
						status, err := json.Marshal(entry.Status)
						t.Fatalf("%s backend connection failed: %s (encode error=%v)", feature, status, err)
					}
				}
			}
			if time.Now().After(deadline) {
				status, err := json.Marshal(servers)
				t.Fatalf("%s backend failed to become ready: tools=%+v servers=%s (encode error=%v)", feature, tools, status, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
