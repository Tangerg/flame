package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestFirstPartyInitializationAndRestartPreserveUserDecisions(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
	cfg := Config{DataDirectory: testsupport.TempDirectory(t), UserHomePath: testsupport.TempDirectory(t), ConfigDirectories: []string{testsupport.TempDirectory(t)}}
	current, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := current.Close(); err != nil {
			t.Error(err)
		}
	})
	initial, err := current.ListPlugins(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Data) != 10 {
		t.Fatalf("default catalog = %d installations", len(initial.Data))
	}
	ids := map[string]string{}
	views := 0
	var memory, diagnostics, usage protocol.PluginInstallation
	for _, item := range initial.Data {
		ids[item.Selected.Name] = item.ID
		if item.Enabled != true || item.Presentation != protocol.PluginPresentationAdmitted {
			t.Fatalf("default installation = %+v", item)
		}

		if item.Selected.Name == "flame.memory" {
			memory = item
		}
		if item.Selected.Name == "flame.diagnostics" {
			diagnostics = item
		}
		if item.Selected.Name == "flame.usage" {
			usage = item
		}
	}
	if views != 8 {
		t.Fatalf("default catalog = %d workspace views", views)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		servers, err := current.ListMCPServers(t.Context(), CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ready := 0
		for _, server := range servers.Data {
			if server.Status.Type == protocol.MCPServerFailed || server.Status.Type == protocol.MCPServerNeedsAuth {
				t.Fatalf("default backend failed: %+v", server)
			}
			if server.Status.Type == protocol.MCPServerConnected {
				ready++
			}
		}
		if ready == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("default backends did not connect: %+v", servers)
		}
		time.Sleep(10 * time.Millisecond)
	}
	schedules, err := current.ListSchedules(t.Context(), protocol.PageQuery{}, CallOptions{})
	if err != nil || len(schedules.Data) != 0 {
		t.Fatalf("default schedule backend = (%v, %v)", schedules, err)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: memory.ID, Enabled: false}, CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: usage.ID, Enabled: false}, CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := current.UninstallPlugin(t.Context(), protocol.PluginRequest{InstallationID: diagnostics.ID}, CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	current, err = Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := current.ListPlugins(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Data) != 9 {
		t.Fatalf("restart restored an uninstalled default: %d", len(restarted.Data))
	}
	for _, item := range restarted.Data {
		if ids[item.Selected.Name] != item.ID {
			t.Fatal("restart duplicated a default installation")
		}
		if item.Selected.Name == "flame.memory" && item.Enabled != false {
			t.Fatal("restart reenabled disabled memory")
		}
		if item.Selected.Name == "flame.usage" && item.Enabled != false {
			t.Fatal("restart restored revoked trust")
		}
	}
	for _, item := range restarted.Data {
		if err := current.UninstallPlugin(t.Context(), protocol.PluginRequest{InstallationID: item.ID}, CommandOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	current, err = Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := current.ListPlugins(t.Context(), CallOptions{})
	if err != nil || len(empty.Data) != 0 {
		t.Fatalf("restart restored deliberately removed defaults: (%v, %v)", empty, err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flame.db", "flame.db-wal", "flame.db-shm"} {
		if err := os.Remove(filepath.Join(cfg.DataDirectory, name)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	current, err = Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := current.ListPlugins(t.Context(), CallOptions{})
	if err != nil || len(reset.Data) != 10 {
		t.Fatalf("empty database did not receive defaults: (%v, %v)", reset, err)
	}
}
