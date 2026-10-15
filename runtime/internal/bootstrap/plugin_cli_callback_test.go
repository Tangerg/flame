package bootstrap

import (
	"bytes"
	json "encoding/json/v2"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	scheduledomain "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestExecutableCLIPluginCommandCallsTheHostWithoutAPage(t *testing.T) {
	cfg := poMCPConfig(t)
	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	info, err := api.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := host.application.newDeliveryEndpoint(t.Context(), info.ServerInfo, cfg.Stores.IdempotencyNamespace.String())
	if err != nil {
		_ = host.Close()
		t.Fatal(err)
	}
	r := &poMCPRuntime{t: t, host: host, endpoint: endpoint}
	defer r.close()
	if err := host.application.recoverUncertainExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	discovery := r.must(delivery.RuntimeDiscover, struct{}{}, "").(*protocol.DiscoverResponse)
	transport, err := flamehttp.NewServer(flamehttp.Config{Endpoint: r.endpoint, Addr: "127.0.0.1:0", ServerInfo: discovery.ServerInfo, LocalToken: "cli-plugin-acceptance-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	httpServer := httptest.NewServer(transport.Handler())
	defer httpServer.Close()
	directory := t.TempDir()
	binary := filepath.Join(directory, "flame")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	build.Dir = filepath.Join("..", "..", "..", "cli")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %s, %v", output, err)
	}
	config := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(config, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(request protocol.CallPluginRequest, key string) string {
		t.Helper()
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(t.Context(), binary, "--config", config, "--runtime-url", httpServer.URL, "plugins", "call", "--request", string(encoded), "--command-id", key)
		command.Dir = directory
		command.Env = append(os.Environ(), "FLAME_HOME="+filepath.Join(directory, "home"), "FLAME_RUNTIME_TOKEN=cli-plugin-acceptance-token")
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			t.Fatalf("CLI business command: %s, %s, %v", output, stderr.String(), err)
		}
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil || reply.Failure != nil || reply.Output == "" {
			t.Fatalf("CLI lost the private business result: %s, %v", output, err)
		}
		return reply.Output
	}
	creation := call(scheduleCreateRequest(r, scheduledomain.Draft{Title: "CLI callback", Instructions: "Complete the original captured work.", CWD: cfg.DefaultWorkspacePath, Cron: "0 0 1 1 *"}), "cli_"+strings.Repeat("8", 32))
	var saved scheduledomain.Snapshot
	if err := json.Unmarshal([]byte(creation), &saved); err != nil || saved.ID == "" {
		t.Fatalf("CLI did not create the private Schedule: %s, %v", creation, err)
	}
	request := scheduleRunRequest(r, saved.ID)
	key := "cli_" + strings.Repeat("9", 32)
	original := call(request, key)
	var handle struct{ SessionID, RunID string }
	if err := json.Unmarshal([]byte(original), &handle, json.RejectUnknownMembers(true)); err != nil || handle.SessionID == "" || handle.RunID == "" {
		t.Fatalf("backend callback did not return an accepted Core Run: %s, %v", original, err)
	}
	if replay := call(request, key); replay != original {
		t.Fatalf("CLI repeated the command with a new Run: original=%s replay=%s", original, replay)
	}
	accepted := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: handle.RunID}, "").(*protocol.RunRef)
	if accepted.ID != handle.RunID || accepted.SessionID != handle.SessionID {
		t.Fatalf("private callback receipt differs from the Runtime owner: %+v", accepted)
	}
}
