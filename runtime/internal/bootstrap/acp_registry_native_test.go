//go:build darwin || linux

package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	framecall "github.com/Tangerg/frame/call"
)

const piRegistryVersion = "0.0.35"

type piSessionObservation struct {
	nativeOpenCodeSession
	AgentID      string
	Capabilities *struct{ List, Load, Resume, Fork bool }
	Events       []struct{ Kind, Text string }
}

func TestACPRegistryPiChecksOriginalSessionPermissionAndCancellation(t *testing.T) {
	adapter, pi := os.Getenv("FLAME_PI_ACP_NATIVE_EXECUTABLE"), os.Getenv("FLAME_PI_NATIVE_EXECUTABLE")
	if adapter == "" || pi == "" {
		t.Skip("set the Pi ACP Registry executable and Pi binary to run their installed lifecycle")
	}
	cwd, launcher, pendingModel := preparePiRegistryFixture(t, adapter, pi)
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = cwd
	r := poMCPOpen(t, cfg)
	installation := r.installAndInspect(filepath.Join(firstPartyPackages, "opencode"), "install-acp-registry")
	profiles := fmt.Sprintf(`[{"id":"pi","name":"Pi","executable":%q,"nativeName":"pi-acp","registry":"pi-acp","version":%q}]`, launcher, piRegistryVersion)
	r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{
		InstallationID: installation.ID, Digest: installation.Selected.Digest,
		ValueChanges: map[string]protocol.PluginValueChange{"agents": {Type: protocol.PluginValueSet, Value: &profiles}},
	}, "configure-pi-registry")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installation.ID, Enabled: true}, "enable-acp-registry")
	waitCallContributorState(t, r, installation.ID, protocol.MCPServerConnected)
	contract, err := plugincontract.ParseCapability([]byte(installation.Selected.Contributions[0].Contract))
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(method, key string, input any) *protocol.CallPluginResponse {
		t.Helper()
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: "native-sessions",
				Contract: contract.Identity().String(), Method: "flame.opencode/" + method + ".v1",
				Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.ID},
			}, Input: string(raw),
		}, key).(*protocol.CallPluginResponse)
	}
	call := func(method, key string, input any) piSessionObservation {
		t.Helper()
		response := invoke(method, key, input)
		var value piSessionObservation
		decodePiACP(t, response, &value)
		return value
	}
	var catalog struct {
		Agents []struct{ ID, Version string }
	}
	decodePiACP(t, invoke("agents", "", struct{}{}), &catalog)
	if len(catalog.Agents) != 2 || catalog.Agents[0].ID != "opencode" || catalog.Agents[1].ID != "pi" || catalog.Agents[1].Version != piRegistryVersion {
		t.Fatalf("native ACP agent catalogue omitted a configured profile: %+v", catalog)
	}
	target := map[string]string{"cwd": cwd, "agentId": "pi"}
	created := call("create", "create-pi", target)
	if created.ID == "" || created.NativeID == "" || !created.Connected || created.AgentID != "pi" ||
		created.Capabilities == nil || !created.Capabilities.List || !created.Capabilities.Load ||
		created.Capabilities.Resume || created.Capabilities.Fork || created.CanFork {
		t.Fatalf("Pi receipt does not retain native identity and advertised capabilities: %+v", created)
	}
	if replay := call("create", "create-pi", target); replay.ID != created.ID || replay.NativeID != created.NativeID {
		t.Fatalf("native creation replay allocated another session: %+v", replay)
	}
	ref := map[string]string{"id": created.ID}
	if fork := invoke("fork", "unsupported-pi-fork", ref); fork.Failure == nil || fork.Failure.Code != "native_session_unavailable" {
		t.Fatalf("Pi's absent fork was advertised or attempted: %+v", fork)
	}
	read := func() piSessionObservation { return call("get", "", ref) }
	begin := call("prompt", "pi-read-with-permission", map[string]string{"id": created.ID, "text": "Read the fixture after permission."})
	waiting := waitPiACP(t, read, "Pi native permission", func(value piSessionObservation) bool {
		return len(value.Permissions) > 0 && value.Permissions[0].Status == "requested"
	})
	permission := waiting.Permissions[0]
	choice := slices.IndexFunc(permission.Options, func(option struct{ ID, Kind string }) bool { return option.Kind == "allow_once" })
	if choice < 0 {
		t.Fatalf("Pi did not return an original permission choice: %+v", permission)
	}
	call("permission", "approve-pi-read", map[string]string{"id": created.ID, "permissionId": permission.ID, "optionId": permission.Options[choice].ID})
	finished := waitPiACP(t, read, "authorized Pi prompt", func(value piSessionObservation) bool {
		return value.Turn != nil && value.Turn.ID == begin.Turn.ID && value.Turn.StopReason == "end_turn"
	})
	if finished.Permissions[0].ID != permission.ID || finished.Permissions[0].Status != "closed" {
		t.Fatalf("the original permission failed to settle with its turn: %+v", finished.Permissions)
	}
	var listed struct{ Rows []struct{ ID string } }
	decodePiACP(t, invoke("list", "", target), &listed)
	if len(listed.Rows) != 1 || listed.Rows[0].ID != created.NativeID {
		t.Fatalf("the Pi process did not retain exactly the native session: %+v", listed)
	}

	cancelling := call("prompt", "pi-prompt-to-cancel", map[string]string{"id": created.ID, "previousTurn": finished.Turn.ID, "text": "Wait for the original cancellation."})
	select {
	case <-pendingModel:
	case <-time.After(15 * time.Second):
		t.Fatal("Pi's native provider never received the original prompt")
	}
	request := call("cancel", "cancel-original-pi", map[string]string{"id": created.ID, "turnId": cancelling.Turn.ID})
	if !request.Turn.CancelRequested || request.Turn.StopReason != "" {
		t.Fatalf("delivery was confused with the original cancellation result: %+v", request)
	}
	waitPiACP(t, read, "Pi native terminal cancellation", func(value piSessionObservation) bool {
		return value.Turn != nil && value.Turn.ID == cancelling.Turn.ID && value.Turn.StopReason == "cancelled"
	})
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installation.ID, protocol.MCPServerConnected)
	rebound := call("open", "reopen-pi-by-original-id", map[string]string{"id": created.ID, "mode": "load"})
	if rebound.ID != created.ID || rebound.NativeID != created.NativeID || rebound.CWD != cwd ||
		!rebound.Connected || rebound.Turn == nil || rebound.Turn.ID != cancelling.Turn.ID || rebound.Turn.StopReason != "cancelled" ||
		!slices.ContainsFunc(rebound.Events, func(event struct{ Kind, Text string }) bool {
			return strings.Contains(event.Text, "Pi inspected the approved fixture.")
		}) {
		t.Fatalf("Pi did not restore the native history and original receipt after restart: %+v", rebound)
	}
	refusal := invoke("open", "pi-refuse-other-workspace", map[string]string{"cwd": t.TempDir(), "agentId": "pi", "nativeId": created.NativeID})
	if refusal.Failure == nil || refusal.Failure.Code != "native_session_unavailable" {
		t.Fatalf("the same native identity attached to another workspace: %+v", refusal)
	}
	program, err := os.ReadFile(launcher)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, append(program, '\n'), 0700); err != nil {
		t.Fatal(err)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installation.ID, protocol.MCPServerConnected)
	changed := invoke("open", "pi-refuse-replaced-program", ref)
	if changed.Failure == nil || changed.Failure.Code != "native_session_unavailable" {
		t.Fatalf("the original Session attached to a replaced executable: %+v", changed)
	}
}

func decodePiACP(t *testing.T, response *protocol.CallPluginResponse, target any) {
	t.Helper()
	if response.Failure != nil {
		t.Fatalf("ACP operation failed: %+v", response.Failure)
	}
	if err := json.Unmarshal([]byte(response.Output), target); err != nil {
		t.Fatal(err)
	}
}

func waitPiACP(t *testing.T, read func() piSessionObservation, phase string, ready func(piSessionObservation) bool) piSessionObservation {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		last := read()
		if ready(last) {
			return last
		}
		select {
		case <-deadline.C:
			t.Fatalf("%s remained pending: turn=%+v permissions=%+v events=%+v", phase, last.Turn, last.Permissions, last.Events)
		case <-tick.C:
		}
	}
}

func preparePiRegistryFixture(t *testing.T, adapter, pi string) (string, string, <-chan struct{}) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil || !filepath.IsAbs(adapter) || !filepath.IsAbs(pi) {
		t.Fatalf("Pi native test requires Node and absolute executables: %v", err)
	}
	pending := make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		servePiFixture(t, pending, w, request)
	}))
	t.Cleanup(provider.Close)
	work := filepath.Join(base, "work")
	piDir := filepath.Join(base, ".pi", "agent")
	extensions := filepath.Join(piDir, "extensions")
	for _, dir := range []string{work, extensions} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	launcher := filepath.Join(base, "pi-acp-fixture")
	for path, body := range map[string]string{
		launcher: "#!/bin/sh\nexec env -i PATH=" + quote(filepath.Dir(node)+":/usr/bin:/bin") + " HOME=" + quote(base) +
			" PI_OFFLINE=1 PI_CODING_AGENT_DIR=" + quote(piDir) + " PI_ACP_PI_COMMAND=" + quote(pi) + " " + quote(adapter) + " \"$@\"\n",
		filepath.Join(piDir, "settings.json"): `{"defaultProvider":"fixture","defaultModel":"synthetic","quietStartup":true}`,
		filepath.Join(piDir, "models.json"):   fmt.Sprintf(`{"providers":{"fixture":{"baseUrl":%q,"api":"openai-completions","apiKey":"loopback","models":[{"id":"synthetic","name":"Fixture","contextWindow":8192,"maxTokens":512}]}}}`, provider.URL+"/v1"),
		filepath.Join(extensions, "require-permission.ts"): `export default function (pi) {
  pi.on("tool_call", async (event, context) => {
    if (event.toolName === "read" && !await context.ui.confirm("Read the fixture", "sample.txt"))
      return { block: true, reason: "Permission declined" };
  });
}`,
		filepath.Join(work, "sample.txt"): "Native Pi fixture\n",
	} {
		mode := os.FileMode(0600)
		if path == launcher {
			mode = 0700
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	return work, launcher, pending
}

func servePiFixture(t *testing.T, pending chan<- struct{}, w http.ResponseWriter, request *http.Request) {
	var input struct {
		Messages []struct {
			Role    string
			Content json.RawMessage
		}
	}
	if request.URL.Path != "/v1/chat/completions" || json.NewDecoder(request.Body).Decode(&input) != nil || len(input.Messages) == 0 {
		t.Errorf("unexpected Pi loopback provider request: %s", request.URL.Path)
		http.Error(w, "invalid fixture input", http.StatusBadRequest)
		return
	}
	last := input.Messages[len(input.Messages)-1]
	w.Header().Set("Content-Type", "text/event-stream")
	if strings.Contains(string(last.Content), "Wait for the original cancellation.") {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case pending <- struct{}{}:
		default:
		}
		select {
		case <-request.Context().Done():
		case <-t.Context().Done():
		}
		return
	}
	chunk := map[string]any{"role": "assistant", "content": "Pi inspected the approved fixture."}
	finish := "stop"
	if last.Role == "user" && strings.Contains(string(last.Content), "Read the fixture after permission.") {
		chunk = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "read_fixture", "type": "function", "function": map[string]string{"name": "read", "arguments": `{"path":"sample.txt"}`}}}}
		finish = "tool_calls"
	}
	for _, choice := range []map[string]any{
		{"index": 0, "delta": chunk, "finish_reason": nil},
		{"index": 0, "delta": map[string]any{}, "finish_reason": finish},
	} {
		event, _ := json.Marshal(map[string]any{"id": "pi-completion", "object": "chat.completion.chunk", "model": "synthetic", "created": 1, "choices": []any{choice}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}
