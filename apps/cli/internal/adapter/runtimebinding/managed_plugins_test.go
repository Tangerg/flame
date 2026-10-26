package runtimebinding

import (
	json "encoding/json/v2"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func activateManagedPlugins(t *testing.T, connection *Connection, features ...string) {
	t.Helper()
	catalog, err := connection.plugins.ListPlugins(t.Context(), connection.callOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range features {
		var installed protocol.PluginInstallation
		for _, item := range catalog.Data {
			if item.Selected.Name == "flame."+feature {
				installed = item
				break
			}
		}
		if installed.ID == "" || installed.Enabled != true {
			t.Fatalf("default %s is unavailable: %+v", feature, installed)
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
