package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

func activateHookProvider(t *testing.T, api *delivery.Handler) *protocol.PluginInstallation {
	t.Helper()
	ctx := protocolLifecycleContext(t.Context())
	installed, err := api.InstallPlugin(ctx, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "hooks")})
	if err != nil {
		t.Fatal(err)
	}
	installed, err = api.SetPluginEnablement(ctx, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.SelectPluginProvider(ctx, protocol.SelectPluginProviderRequest{Role: "flame.hooks.v1", InstallationID: installed.ID, Contribution: "hook-provider"}); err != nil {
		t.Fatal(err)
	}
	return installed
}

func writeHookSource(t *testing.T, root string, rules string) {
	t.Helper()
	directory := filepath.Join(root, ".flame")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "hooks.json"), []byte(rules), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledHooksRetainRulesAndOriginalCommandReceipt(t *testing.T) {
	cfg := poMCPConfig(t)
	if err := os.Mkdir(filepath.Join(cfg.defaultWorkspacePath, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	command := `printf 'once\n' >> hook-effects; printf '%s' '{"decision":"ask","reason":"confirm original","injectContext":"captured"}'`
	document, err := json.Marshal(map[string]any{"hooks": []any{map[string]string{"event": "PreToolUse", "matcher": "shell", "command": command}}})
	if err != nil {
		t.Fatal(err)
	}
	writeHookSource(t, cfg.userHome, string(document))
	r := poMCPOpen(t, cfg)
	installed := activateHookProvider(t, r.api)
	source := r.host.application.delivery.PluginCalls.(adapterhooks.Source)
	resolver, err := newHookResolver(cfg.userHome, cfg.stores.Trust, source)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), cfg.defaultWorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bound.Close() })
	writeHookSource(t, cfg.userHome, `{"hooks":[{"event":"PreToolUse","matcher":"shell","inject":"replacement"}]}`)
	input := hook.Input{Event: hook.PreToolUse, SessionID: "session:hooks", CWD: cfg.defaultWorkspacePath, Tool: &hook.ToolInput{Name: "shell", Arguments: `{"command":"echo original"}`}}
	for range 2 {
		decision := bound.Run(t.Context(), input)
		if decision.Verdict != domainhooks.VerdictAsk || decision.Reason != "confirm original" || decision.InjectContext != "captured" {
			t.Fatalf("captured command changed its result: %+v", decision)
		}
	}
	if content, err := os.ReadFile(filepath.Join(cfg.defaultWorkspacePath, "hook-effects")); err != nil || string(content) != "once\n" {
		t.Fatalf("original command was redispatched: %q %v", content, err)
	}
	blocked := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, delivery.Options{IdempotencyKey: "remove-held-hooks", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if blocked.Failure == nil || !errors.Is(blocked.Failure, protocol.ErrPluginInUse) {
		t.Fatalf("captured source was not retained: %v", blocked.Failure)
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := resolver.For(t.Context(), cfg.defaultWorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if decision := current.Run(t.Context(), input); decision.InjectContext != "replacement" {
		t.Fatalf("new discovery did not see the edited source: %+v", decision)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "remove-released-hooks")
}

func TestInstalledHooksAbsenceTrustAndDisablementThroughRuntime(t *testing.T) {
	cfg := poMCPConfig(t)
	if err := os.Mkdir(filepath.Join(cfg.defaultWorkspacePath, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	writeHookSource(t, cfg.userHome, `{"hooks":[{"event":"UserPromptSubmit","command":"printf x > global-hook-effect; printf '%s' '{\"decision\":\"deny\",\"reason\":\"global policy\"}'"}]}`)
	writeHookSource(t, cfg.defaultWorkspacePath, "{")
	r := poMCPOpen(t, cfg)
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "optional hook source"}, "hook-session").(*protocol.Session)
	start := func(key string) delivery.Result {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "continue"}}}, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if result.Events != nil {
			for _, err := range result.Events {
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		return result
	}
	if result := start("without-hook-provider"); result.Failure != nil {
		t.Fatalf("absent optional hooks prevented execution: %v", result.Failure)
	}
	installed := activateHookProvider(t, r.api)
	if result := start("global-policy"); result.Failure == nil {
		t.Fatal("installed global policy did not block the prompt")
	}
	if content, err := os.ReadFile(filepath.Join(cfg.defaultWorkspacePath, "global-hook-effect")); err != nil || string(content) != "x" {
		t.Fatalf("untrusted malformed project prevented the global command: %q %v", content, err)
	}
	if _, err := r.api.ListHooks(protocolLifecycleContext(t.Context()), protocol.ListHooksRequest{}); err == nil {
		t.Fatal("management inspection silently discarded malformed project configuration")
	}
	r.must(delivery.Name("hooks.setTrust"), protocol.SetHookTrustRequest{ProjectRoot: cfg.defaultWorkspacePath, Trusted: true}, "trust-hook-project")
	writeHookSource(t, cfg.defaultWorkspacePath, `{"hooks":[{"event":"UserPromptSubmit","inject":"trusted project"}]}`)
	view, err := r.api.ListHooks(protocolLifecycleContext(t.Context()), protocol.ListHooksRequest{})
	if err != nil || !view.ProjectTrusted || len(view.Hooks) != 2 {
		t.Fatalf("installed rules lost Runtime trust: %+v %v", view, err)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-hooks")
	if result := start("without-enabled-hooks"); result.Failure != nil {
		t.Fatalf("disabled hooks still contributed rules: %v", result.Failure)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	view, err = r.api.ListHooks(protocolLifecycleContext(context.Background()), protocol.ListHooksRequest{})
	if err != nil || !view.ProjectTrusted || len(view.Hooks) != 0 {
		t.Fatalf("disabled source or trust changed across restart: %+v %v", view, err)
	}
	r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: session.ID}, "")
}

func TestInstalledHookCancellationStopsItsCommandGroup(t *testing.T) {
	cfg := poMCPConfig(t)
	writeHookSource(t, cfg.userHome, `{"hooks":[{"event":"Stop","command":"printf x > hook-started; (sleep 0.4; touch hook-escaped) & wait"}]}`)
	r := poMCPOpen(t, cfg)
	activateHookProvider(t, r.api)
	resolver, err := newHookResolver(cfg.userHome, cfg.stores.Trust, r.host.application.delivery.PluginCalls.(adapterhooks.Source))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), cfg.defaultWorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bound.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		bound.Run(ctx, hook.Input{Event: hook.Stop, SessionID: "session:hooks", CWD: cfg.defaultWorkspacePath})
	}()
	deadline := time.After(3 * time.Second)
	for {
		_, err := os.Stat(filepath.Join(cfg.defaultWorkspacePath, "hook-started"))
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-finished:
			t.Fatal("hook finished before its command started")
		case <-deadline:
			t.Fatal("hook command did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("installed command did not observe cancellation")
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(cfg.defaultWorkspacePath, "hook-escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled command left a running descendant: %v", err)
	}
}
