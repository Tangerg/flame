package integration

import (
	json "encoding/json/v2"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/qimu/portable"

	flameruntime "github.com/Tangerg/flame/runtime"
)

func TestFirstPartyInitializationAndRestartPreserveUserDecisions(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
	cfg := flameruntime.Config{DataDirectory: tempRuntimeDirectory(t), UserHomePath: tempRuntimeDirectory(t), ConfigDirectories: []string{tempRuntimeDirectory(t)}}
	current, err := flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := current.Close(); err != nil {
			t.Error(err)
		}
	})
	initial, err := current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := os.ReadDir(os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY"))
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Data) != len(sources) {
		t.Fatalf("default catalog installed %d of %d supplied packages", len(initial.Data), len(sources))
	}
	ids := map[string]string{}
	backendCount := 0
	var memory, diagnostics, usage, scheduling protocol.PluginInstallation
	for _, item := range initial.Data {
		ids[item.Selected.Name] = item.ID
		backendCount += len(item.Selected.Servers)
		if item.Selected.Name == "flame.schedules" {
			scheduling = item
		}
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
	for _, item := range []protocol.PluginInstallation{memory, diagnostics, usage} {
		if item.ID == "" || !slices.ContainsFunc(item.Selected.Components, func(component protocol.PluginClientComponent) bool { return component.Placement == "workspaceView" }) {
			t.Fatalf("required workspace page is absent: %+v", item)
		}
	}
	if scheduling.ID == "" {
		t.Fatal("default schedule backend is absent")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		servers, err := current.ListMCPServers(t.Context(), protocol.CallOptions{})
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
		if ready == backendCount {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("default backends did not connect: %+v", servers)
		}
		time.Sleep(10 * time.Millisecond)
	}
	contract, err := portable.ParseCapability([]byte(SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(SchedulesListV1Input{})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := current.CallPlugin(t.Context(), protocol.CallPluginRequest{InstallationID: scheduling.ID, Digest: scheduling.Selected.Digest, Contribution: "schedules-provider", Contract: string(contract.Identity().JSON()), Method: SchedulesListV1Method, Input: string(input), Target: portable.Target{Scope: "installation", Reference: scheduling.ID}}, protocol.PluginCallOptions{})
	if err != nil || reply.Failure != nil {
		t.Fatalf("default schedule backend: %+v, %v", reply, err)
	}
	var schedules SchedulesListV1Output
	if err := json.Unmarshal([]byte(reply.Output), &schedules); err != nil || len(schedules.Rows) != 0 {
		t.Fatalf("default schedules: %+v, %v", schedules, err)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: memory.ID, Enabled: false}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: usage.ID, Enabled: false}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := current.UninstallPlugin(t.Context(), protocol.PluginRequest{InstallationID: diagnostics.ID}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	current, err = flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Data) != len(initial.Data)-1 {
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
		if err := current.UninstallPlugin(t.Context(), protocol.PluginRequest{InstallationID: item.ID}, protocol.CommandOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	current, err = flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil || len(empty.Data) != 0 {
		t.Fatalf("restart restored deliberately removed defaults: (%v, %v)", empty, err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	layout, err := localruntime.DataDirectoryAt(cfg.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	database := layout.DatabasePath()
	for _, path := range []string{database, database + "-wal", database + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	current, err = flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil || len(reset.Data) != len(initial.Data) {
		t.Fatalf("empty database did not receive defaults: (%v, %v)", reset, err)
	}
}
