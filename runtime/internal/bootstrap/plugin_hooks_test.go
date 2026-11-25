package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
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
	command := `printf 'once\n' >> hook-effects; printf '%s' '{"decision":"ask","reason":"confirm original","injectContext":"captured","rewriteArguments":"{\"command\":\"echo policy\"}"}'`
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
		if decision.Verdict != domainhooks.VerdictAsk || decision.Reason != "confirm original" || decision.InjectContext != "captured" || decision.RewriteArguments != `{"command":"echo policy"}` {
			t.Fatalf("captured command changed its result: %+v", decision)
		}
	}
	if content, err := os.ReadFile(filepath.Join(cfg.defaultWorkspacePath, "hook-effects")); err != nil || string(content) != "once\n" {
		t.Fatalf("original command was redispatched: %q %v", content, err)
	}
	blocked := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, protocol.CallMetadata{IdempotencyKey: "remove-held-hooks", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	start := func(key string) protocol.OperationResult {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "continue"}}}, protocol.CallMetadata{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	r.must(delivery.Name("hooks.setTrust"), protocol.SetHookTrustRequest{RepositoryRoot: cfg.defaultWorkspacePath, Trusted: true}, "trust-hook-project")
	writeHookSource(t, cfg.defaultWorkspacePath, `{"hooks":[{"event":"UserPromptSubmit","inject":"trusted project"}]}`)
	view, err := r.api.ListHooks(protocolLifecycleContext(t.Context()), protocol.ListHooksRequest{})
	if err != nil || !view.RepositoryTrusted || len(view.Hooks) != 2 {
		t.Fatalf("installed rules lost Runtime trust: %+v %v", view, err)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-hooks")
	if result := start("without-enabled-hooks"); result.Failure != nil {
		t.Fatalf("disabled hooks still contributed rules: %v", result.Failure)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	view, err = r.api.ListHooks(protocolLifecycleContext(context.Background()), protocol.ListHooksRequest{})
	if err != nil || !view.RepositoryTrusted || len(view.Hooks) != 0 {
		t.Fatalf("disabled source or trust changed across restart: %+v %v", view, err)
	}
	r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: session.ID}, "")
}

func TestInstalledWaitingHookSurvivesCancelingTheParkedRun(t *testing.T) {
	cfg := poMCPConfig(t)
	model := newLifecycleModel()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	const (
		startedMarker  = "waiting-hook-started"
		finishedMarker = "waiting-hook-finished"
		releasePipe    = "waiting-hook-release"
	)
	workspace := cfg.defaultWorkspacePath
	pipePath := filepath.Join(workspace, releasePipe)
	if err := unix.Mkfifo(pipePath, 0600); err != nil {
		t.Fatal(err)
	}
	pipe, err := os.OpenFile(pipePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	command := `printf 'started' > ` + startedMarker +
		`; IFS= read -r release < ` + releasePipe +
		`; printf 'finished' > ` + finishedMarker + `; printf '%s' '{}'`
	rules, err := json.Marshal(map[string]any{"hooks": []any{map[string]string{
		"event":   string(hook.Notification),
		"command": command,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeHookSource(t, cfg.userHome, string(rules))
	runtime := poMCPOpen(t, cfg)
	releaseModel := sync.OnceFunc(func() { close(model.releaseFirstCall) })
	t.Cleanup(releaseModel)
	activateHookProvider(t, runtime.api)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := watcher.Add(workspace); err != nil {
		t.Fatal(err)
	}
	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "waiting notification",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Ask a question."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	parked := collectRunEvents(events)
	waitForSignal(t, model.firstCallStarted, "the first model call")
	releaseModel()
	waitForRunEvents(t, parked, "the completed park")
	pending, err := runtime.api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("the notification has no committed interrupt: %+v, %v", pending, err)
	}
	awaitStart, cancelStart := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancelStart()
	if !observeWaitingHook(t, awaitStart, watcher, filepath.Join(workspace, startedMarker)) {
		t.Fatal("the committed waiting boundary did not reach the installed hook")
	}

	canceled, err := runtime.api.CancelRun(ctx, protocol.CancelRunRequest{
		RunID: started.RunID, Reason: "Cancel the parked Run during its notification",
	})
	if err != nil || canceled.Run.Outcome == nil || canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("the running notification blocked cancellation: %+v, %v", canceled, err)
	}
	if _, err := io.WriteString(pipe, "continue\n"); err != nil {
		t.Fatal(err)
	}
	awaitFinishing, cancelFinishing := context.WithTimeout(ctx, 3*time.Second)
	defer cancelFinishing()
	if !observeWaitingHook(t, awaitFinishing, watcher, filepath.Join(workspace, finishedMarker)) {
		t.Fatal("retiring the parked executor stopped delivery of its committed waiting notification")
	}
}

func observeWaitingHook(t *testing.T, ctx context.Context, watcher *fsnotify.Watcher, marker string) bool {
	t.Helper()
	for {
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				t.Fatal(err)
			}
			return true
		}
		select {
		case <-watcher.Events:
		case err := <-watcher.Errors:
			t.Fatalf("observe the installed waiting hook: %v", err)
		case <-ctx.Done():
			return false
		}
	}
}
