//go:build darwin || linux

package bootstrap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	framecall "github.com/Tangerg/frame/call"
)

type nativePermission struct {
	ID      string
	Status  string
	Options []struct {
		ID   string
		Kind string
	}
}

type nativeOpenCodeSession struct {
	ID, NativeID, Agent, Machine, CWD string
	Connected, CanFork                bool
	Turn                              *struct {
		ID, Status, StopReason string
		CancelRequested        bool
	}
	Permissions []nativePermission
}

func TestOpenCodeHostKeepsTheNativeSessionPermissionsAndCancellation(t *testing.T) {
	executable := os.Getenv("FLAME_OPENCODE_NATIVE_EXECUTABLE")
	if executable == "" {
		t.Skip("set FLAME_OPENCODE_NATIVE_EXECUTABLE to exercise installed OpenCode offline")
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(base, "work")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "approved.txt")
	provider := httptest.NewServer(nativeFixtureModel{path: path})
	t.Cleanup(provider.Close)
	shellValue := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	pidFile := filepath.Join(base, "acp.pid")
	launcher := filepath.Join(base, "open-code-fixture")
	body := "#!/bin/sh\n" +
		"printf '%s' \"$$\" > " + shellValue(pidFile) + "\n" +
		"exec env -i PATH=/usr/bin:/bin HOME=" + shellValue(base) +
		" XDG_CONFIG_HOME=" + shellValue(filepath.Join(base, "config")) +
		" XDG_DATA_HOME=" + shellValue(filepath.Join(base, "data")) +
		" XDG_STATE_HOME=" + shellValue(filepath.Join(base, "state")) +
		" XDG_CACHE_HOME=" + shellValue(filepath.Join(base, "cache")) +
		" OPENCODE_DISABLE_AUTOUPDATE=true OPENCODE_DISABLE_MODELS_FETCH=true OPENCODE_DISABLE_DEFAULT_PLUGINS=true " +
		shellValue(executable) + " acp\n"
	if err := os.WriteFile(launcher, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	opencodeConfig := map[string]any{
		"model": "fixture/synthetic", "small_model": "fixture/synthetic",
		"provider": map[string]any{"fixture": map[string]any{
			"name": "Local ACP fixture", "npm": "@ai-sdk/openai-compatible",
			"options": map[string]string{"baseURL": provider.URL + "/v1", "apiKey": "test-only"},
			"models":  map[string]any{"synthetic": map[string]string{"name": "Synthetic"}},
		}},
		"permission": map[string]string{"edit": "ask", "bash": "deny", "webfetch": "deny", "websearch": "deny"},
	}
	config, err := json.Marshal(opencodeConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "opencode.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--quiet", "--template="}, {"add", "opencode.json"}, {"commit", "--quiet", "-m", "OpenCode fixture"},
	} {
		git := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "core.hooksPath=" + os.DevNull}, args...)...)
		git.Dir = cwd
		if raw, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, raw, err)
		}
	}
	native := seedOpenCodeSession(t, launcher, cwd)
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = cwd
	r := poMCPOpen(t, cfg)
	installation := r.installAndInspect(filepath.Join(firstPartyPackages, "opencode"), "install-opencode")
	r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{
		InstallationID: installation.ID, Digest: installation.Selected.Digest,
		ValueChanges: map[string]protocol.PluginValueChange{
			"executable": {Type: protocol.PluginValueSet, Value: &launcher},
		},
	}, "configure-opencode")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installation.ID, Enabled: true}, "enable-opencode")
	waitCallContributorState(t, r, installation.ID, protocol.MCPServerConnected)

	reply := func(method, command string, input any) *protocol.CallPluginResponse {
		t.Helper()
		current := r.pluginInstallation(installation.ID)
		contract, err := plugincontract.ParseCapability([]byte(current.Selected.Contributions[0].Contract))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: current.ID, Digest: current.Selected.Digest, Contribution: "native-sessions",
				Contract: contract.Identity().String(), Method: "flame.opencode/" + method + ".v1",
				Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: current.ID},
			}, Input: string(raw),
		}, command).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("%s: %+v", method, result.Failure)
		}
		return result
	}
	call := func(method, command string, input any) nativeOpenCodeSession {
		t.Helper()
		var out nativeOpenCodeSession
		if err := json.Unmarshal([]byte(reply(method, command, input).Output), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	list := reply("list", "", map[string]string{"cwd": cwd})
	if !strings.Contains(list.Output, native) {
		t.Fatalf("native list did not find the existing session: %s", list.Output)
	}
	openInput := map[string]string{"cwd": cwd, "nativeId": native}
	got := call("open", "open-original", openInput)
	if !got.Connected || got.NativeID != native || got.CWD != cwd || got.Machine == "" || !strings.HasPrefix(got.Agent, "OpenCode") {
		t.Fatalf("incorrect native binding: %+v", got)
	}
	ref := map[string]string{"id": got.ID}
	read := func() nativeOpenCodeSession { return call("get", "", ref) }
	await := func(label string, condition func(nativeOpenCodeSession) bool) nativeOpenCodeSession {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			current := read()
			if condition(current) {
				return current
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %+v", label, current)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	openTurn := func(key, previous, text string) nativeOpenCodeSession {
		t.Helper()
		return call("prompt", key, map[string]string{"id": got.ID, "previousTurn": previous, "text": text})
	}
	pending := func(current nativeOpenCodeSession) bool {
		return slices.ContainsFunc(current.Permissions, func(permission nativePermission) bool { return permission.Status == "requested" })
	}
	currentPermission := func(current nativeOpenCodeSession) nativePermission {
		t.Helper()
		i := slices.IndexFunc(current.Permissions, func(permission nativePermission) bool { return permission.Status == "requested" })
		if i < 0 {
			t.Fatal("the original permission is missing")
		}
		return current.Permissions[i]
	}
	first := openTurn("prompt-for-edit", "", "first edit")
	parked := await("native write permission", pending)
	permission := currentPermission(parked)
	if observed := currentPermission(read()); observed.ID != permission.ID {
		t.Fatal("a new observer did not reopen the original permission")
	}
	choice := slices.IndexFunc(permission.Options, func(option struct{ ID, Kind string }) bool { return option.Kind == "allow_once" })
	if choice < 0 {
		t.Fatal("OpenCode did not offer allow_once")
	}
	answer := map[string]string{"id": got.ID, "permissionId": permission.ID, "optionId": permission.Options[choice].ID}
	call("permission", "allow-original", answer)
	call("permission", "allow-original", answer)
	done := await("approved native prompt", func(value nativeOpenCodeSession) bool {
		return value.Turn != nil && value.Turn.ID == first.Turn.ID && value.Turn.StopReason == "end_turn"
	})
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "OpenCode ACP acceptance\n" {
		t.Fatalf("approved edit was not performed by the native agent: %q, %v", raw, err)
	}
	diff := r.must(delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: cwd}, Format: protocol.DiffFormatRaw,
	}, "").(*protocol.Diff)
	if diff.Patch == nil || !strings.Contains(*diff.Patch, "+OpenCode ACP acceptance") {
		t.Fatalf("Runtime did not read the agent's actual workspace diff: %+v", diff)
	}
	if got.CanFork {
		branch := call("fork", "fork-original", ref)
		if branch.NativeID == native || branch.NativeID == "" || branch.CWD != cwd {
			t.Fatalf("advertised fork did not create a native session: %+v", branch)
		}
		replay := call("fork", "fork-original", ref)
		if replay.NativeID != branch.NativeID {
			t.Fatalf("fork replay created a second native session: %+v", replay)
		}
	}
	second := openTurn("prompt-then-cancel", done.Turn.ID, "second edit")
	await("permission before cancel", pending)
	cancel := map[string]string{"id": got.ID, "turnId": second.Turn.ID}
	if requested := call("cancel", "cancel-original", cancel); !requested.Turn.CancelRequested || requested.Turn.StopReason != "" {
		t.Fatalf("cancellation request was reported as already settled: %+v", requested.Turn)
	}
	await("original prompt cancellation confirmed by OpenCode", func(value nativeOpenCodeSession) bool {
		return value.Turn != nil && value.Turn.ID == second.Turn.ID && value.Turn.StopReason == "cancelled"
	})

	third := openTurn("prompt-before-disconnect", second.Turn.ID, "third edit")
	lost := currentPermission(await("permission before disconnect", pending)).ID
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	await("original process disconnected", func(value nativeOpenCodeSession) bool {
		return !value.Connected && value.Turn != nil && value.Turn.Status == "unconfirmed"
	})
	cancel = map[string]string{"id": got.ID, "turnId": third.Turn.ID}
	offline := call("cancel", "offline-cancel", cancel)
	if !offline.Turn.CancelRequested || offline.Turn.StopReason != "" {
		t.Fatalf("offline cancel invented a remote confirmation: %+v", offline.Turn)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installation.ID, protocol.MCPServerConnected)
	recovered := call("open", "reopen-original", openInput)
	if recovered.ID != got.ID || recovered.NativeID != native || !recovered.Connected ||
		recovered.Turn == nil || recovered.Turn.ID != third.Turn.ID || !recovered.Turn.CancelRequested ||
		recovered.Turn.StopReason != "" ||
		!slices.ContainsFunc(recovered.Permissions, func(entry nativePermission) bool {
			return entry.ID == lost && entry.Status == "unconfirmed"
		}) {
		t.Fatalf("restart did not preserve the original permission and uncertain cancellation: %+v", recovered)
	}
}

func seedOpenCodeSession(t *testing.T, executable, cwd string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	cmd.Dir = cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	reply := bufio.NewScanner(stdout)
	reply.Buffer(make([]byte, 4096), 1<<20)
	send := func(id int, method string, input any) json.RawMessage {
		t.Helper()
		if err := json.NewEncoder(stdin).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": input}); err != nil {
			t.Fatal(err)
		}
		for reply.Scan() {
			var message struct {
				ID     int
				Result json.RawMessage
				Error  json.RawMessage
			}
			if err := json.Unmarshal(reply.Bytes(), &message); err != nil {
				t.Fatal(err)
			}
			if message.ID != id {
				continue
			}
			if len(message.Error) != 0 {
				t.Fatalf("%s returned %s", method, message.Error)
			}
			return message.Result
		}
		t.Fatalf("no response from OpenCode to %s: %v", method, reply.Err())
		return nil
	}
	send(1, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}})
	var created struct{ SessionID string }
	if err := json.Unmarshal(send(2, "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}), &created); err != nil || created.SessionID == "" {
		t.Fatalf("could not seed an existing native session: %+v, %v", created, err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("OpenCode fixture exit: %v: %s", err, &stderr)
	}
	return created.SessionID
}

type nativeFixtureModel struct{ path string }

func (m nativeFixtureModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"synthetic","object":"model"}]}`)
		return
	}
	var request struct {
		Messages []struct {
			Role    string
			Content json.RawMessage
		}
		Tools []struct {
			Function struct{ Name string }
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	latestUser := -1
	for i, message := range request.Messages {
		if message.Role == "user" {
			latestUser = i
		}
	}
	alreadyWrote := slices.ContainsFunc(request.Messages[latestUser+1:], func(message struct {
		Role    string
		Content json.RawMessage
	}) bool {
		return message.Role == "tool"
	})
	hasWrite := slices.ContainsFunc(request.Tools, func(tool struct{ Function struct{ Name string } }) bool { return tool.Function.Name == "write" })
	finish := "stop"
	chunk := map[string]any{"role": "assistant", "content": "Applied the approved workspace edit."}
	if hasWrite && !alreadyWrote {
		finish = "tool_calls"
		arguments, _ := json.Marshal(map[string]string{"filePath": m.path, "content": "OpenCode ACP acceptance\n"})
		chunk = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
			"index": 0, "id": "call_" + strconv.Itoa(latestUser), "type": "function",
			"function": map[string]any{"name": "write", "arguments": string(arguments)},
		}}}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, choice := range []map[string]any{
		{"index": 0, "delta": chunk, "finish_reason": nil},
		{"index": 0, "delta": map[string]any{}, "finish_reason": finish},
	} {
		event, _ := json.Marshal(map[string]any{
			"id": "completion", "object": "chat.completion.chunk", "created": 1, "model": "synthetic", "choices": []any{choice},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}
