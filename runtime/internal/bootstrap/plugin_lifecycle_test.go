package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

func TestPluginReleaseUsesExistingMCPPolicySkillsAndReplay(t *testing.T) {
	var calls atomic.Int32
	remote := sdk.NewServer(&sdk.Implementation{Name: "review", Version: "1"}, nil)
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "record", Description: "Record a review"}, func(_ context.Context, input struct {
		Value string `json:"value"`
	}) (string, error) {
		calls.Add(1)
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scopemcp.Register(remote, executable); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(server.Close)
	cfg := runtimeConfigWithRequiredDeps(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(cfg.Stores.DataDirectory, "plugins"), func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "park", Name: "ask_user", Arguments: `{"questions":[{"question":"Review the pending work?"}]}`}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)

	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}
	invoke := func(method delivery.Name, input any, key string) delivery.Result {
		t.Helper()
		attempt := options
		attempt.IdempotencyKey = key
		return endpoint.Invoke(t.Context(), method, input, attempt)
	}
	must := func(method delivery.Name, input any, key string) any {
		t.Helper()
		result := invoke(method, input, key)
		if result.Failure != nil {
			t.Fatalf("%s: %v (%v)", method, result.Failure, errors.Unwrap(result.Failure))
		}
		return result.Value
	}
	for _, test := range []struct {
		name, source string
	}{
		{"relative", "relative/package"},
		{"missing-manifest", t.TempDir()},
		{"invalid-manifest", writeInvalidPluginManifest(t, `{"$schema":"unsupported","name":"review"}`)},
		{"duplicate-fields", writeInvalidPluginManifest(t, `{"name":"first","name":"second"}`)},
	} {
		result := invoke(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: test.source}, "invalid-install-"+test.name)
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
			t.Fatalf("%s package input lost its validation category: %v", test.name, result.Failure)
		}
	}
	if installed := must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation]); len(installed.Data) != 0 {
		t.Fatal("invalid package input created an installation")
	}
	source := t.TempDir()
	files := map[string]string{
		"plugin.json":            `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"review-package","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"requests":[{"capability":"tools.invoke","targets":["reviews/record"]}]}}}`,
		"mcp.json":               `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":"` + server.URL + `"}}}`,
		"skills/review/SKILL.md": "---\nname: review\ndescription: Review this workspace\n---\nInspect each change.\n",
	}
	for name, body := range files {
		file := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	installed := must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-release").(*protocol.PluginInstallation)
	if installed.Enabled || installed.ApprovedDigest != "" || len(installed.Selected.Skills) != 1 {
		t.Fatalf("unexpected admission: %+v", installed)
	}
	replayed := must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-release").(*protocol.PluginInstallation)
	if replayed.ID != installed.ID {
		t.Fatal("install replay changed identity")
	}
	release := protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}
	staleConfiguration := invoke(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: strings.Repeat("0", 64)}, "configure-other-release")
	if staleConfiguration.Failure == nil || !errors.Is(staleConfiguration.Failure, protocol.ErrPluginStale) {
		t.Fatalf("stale configuration: %v", staleConfiguration.Failure)
	}
	unboundConfiguration := invoke(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID}, "configure-unbound-release")
	if unboundConfiguration.Failure == nil || !errors.Is(unboundConfiguration.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("unbound configuration: %v", unboundConfiguration.Failure)
	}
	must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: release.Digest}, "configure-selected-release")
	denied := invoke(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-unapproved")
	if denied.Failure == nil || !errors.Is(denied.Failure, protocol.ErrPluginUnapproved) {
		t.Fatalf("unapproved enable: %v", denied.Failure)
	}
	must(delivery.PluginsApprove, protocol.ApprovePluginRequest{InstallationID: installed.ID, Digest: release.Digest, Grants: installed.Selected.Requests}, "approve-release")
	must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-release")
	name := "installation/" + installed.ID + "/reviews"
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		tools := must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: name}, "").(*protocol.Page[protocol.MCPTool])
		if len(tools.Data) > 0 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("installation MCP did not connect")
		case <-time.After(10 * time.Millisecond):
		}
	}
	refused := invoke(delivery.MCPServersDelete, protocol.MCPServerRequest{Server: name}, "delete-derived-server")
	if refused.Failure == nil || !errors.Is(refused.Failure, protocol.ErrMCPOwnedByInstallation) {
		t.Fatalf("installation server CRUD: %v", refused.Failure)
	}
	discovered := must(delivery.SkillsDiscoveredList, protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "").(*protocol.SkillDiscovery)
	if len(discovered.Skills) != 1 || discovered.Skills[0].Scope != protocol.SkillScopeInstallation || discovered.Skills[0].Installation == nil || discovered.Skills[0].Installation.InstallationID != installed.ID {
		t.Fatalf("package skill provenance: %+v", discovered)
	}
	selected := must(delivery.SessionsCreate, protocol.CreateSessionRequest{}, "action-session").(*protocol.Session)
	ctx := protocolLifecycleContext(t.Context())
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: selected.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Park with the exact package dependencies."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "plugin dependency wait")
	waiting, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || waiting.Status != protocol.RunStatusWaiting {
		t.Fatalf("waiting package run: %+v, %v", waiting, err)
	}
	if err = os.WriteFile(filepath.Join(source, "notes.txt"), []byte("next release"), 0600); err != nil {
		t.Fatal(err)
	}
	staged := must(delivery.PluginsStage, protocol.StagePluginRequest{InstallationID: installed.ID, Source: source}, "stage-while-waiting").(*protocol.PluginInstallation)
	if staged.Staged == nil || staged.Selected.Digest != release.Digest {
		t.Fatal("staging changed executable selection")
	}
	blocked := invoke(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: staged.Staged.Digest}, "select-while-waiting")
	if blocked.Failure == nil || !errors.Is(blocked.Failure, protocol.ErrPluginInUse) {
		t.Fatalf("waiting dependency switch: %v", blocked.Failure)
	}
	endpoint.BeginShutdown()
	if err = endpoint.AwaitShutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	host, api = buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	endpoint, err = delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	replayed = must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-release").(*protocol.PluginInstallation)
	if replayed.ID != installed.ID {
		t.Fatal("cold replay created another installation")
	}
	blocked = invoke(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: staged.Staged.Digest}, "select-after-restart")
	if blocked.Failure == nil || !errors.Is(blocked.Failure, protocol.ErrPluginInUse) {
		t.Fatalf("cold waiting dependency switch: %v", blocked.Failure)
	}

	must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke-release")

	discovered = must(delivery.SkillsDiscoveredList, protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "").(*protocol.SkillDiscovery)
	if len(discovered.Skills) != 0 {
		t.Fatal("revoked skill remained available")
	}
}

func writeInvalidPluginManifest(t *testing.T, manifest string) string {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	return source
}
