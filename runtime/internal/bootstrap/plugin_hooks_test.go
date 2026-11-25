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
	writeHookSource(t, cfg.defaultWorkspacePath, `{"hooks":[{"event":"UserPromptSubmit","inject":"trusted project"}]}`)
	const trustMethod delivery.Name = "hooks.setTrust"
	const trustKey = "trust-hook-project"
	trust := protocol.SetHookTrustRequest{RepositoryRoot: cfg.defaultWorkspacePath, Trusted: true}
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	refused := r.endpoint.Invoke(t.Context(), trustMethod, trust, protocol.CallMetadata{IdempotencyKey: trustKey})
	allowReceipt()
	if refused.Failure == nil {
		t.Fatal("trusted the repository despite a refused command receipt")
	}
	if view, err := r.api.ListHooks(protocolLifecycleContext(t.Context()), protocol.ListHooksRequest{}); err != nil || view == nil || view.RepositoryTrusted {
		t.Fatalf("repository trust after a refused receipt = %+v, %v, want untrusted", view, err)
	}
	r.must(trustMethod, trust, trustKey)
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
	f := startRunWithHeldHook(t, hook.Notification)
	f.releaseModel()
	waitForRunEvents(t, f.events, "the completed park")
	pending, err := f.runtime.api.ListInterrupts(f.ctx, protocol.ListInterruptsRequest{RootRunID: f.started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("the notification has no committed interrupt: %+v, %v", pending, err)
	}
	f.waitForHookFile(t, f.hookStarted)
	canceled, err := f.runtime.api.CancelRun(f.ctx, protocol.CancelRunRequest{
		RunID: f.started.RunID, Reason: "Cancel the parked Run during its notification",
	})
	if err != nil || canceled.Run.Outcome == nil || canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("the running notification blocked cancellation: %+v, %v", canceled, err)
	}
	f.releaseHook()
	f.waitForHookFile(t, f.hookFinished)
}

func TestInstalledStopHookDoesNotDelayCancellation(t *testing.T) {
	f := startRunWithHeldHook(t, hook.Stop)
	request := protocol.CancelRunRequest{RunID: f.started.RunID, Reason: "Cancel while the stop notification is held"}
	key := protocol.CallMetadata{IdempotencyKey: "cancel-with-stop-hook"}
	replies := make(chan protocol.OperationResult, 1)
	go func() { replies <- f.runtime.endpoint.Invoke(f.ctx, delivery.RunsCancel, request, key) }()
	f.waitForHookFile(t, f.hookStarted)
	stopped, err := f.runtime.api.GetRun(f.ctx, protocol.GetRunRequest{RunID: f.started.RunID})
	if err != nil || stopped.Outcome == nil || stopped.Outcome.Type != protocol.OutcomeCanceled {
		t.Errorf("the stop hook has no committed cancellation: %+v, %v", stopped, err)
	}
	select {
	case reply := <-replies:
		response, ok := reply.Value.(*protocol.CancelRunResponse)
		if reply.Failure != nil || !ok || response.Run.Outcome == nil ||
			response.Run.Outcome.Type != protocol.OutcomeCanceled || response.Run.Outcome.Detail != request.Reason {
			t.Errorf("the public cancellation could not return while the stop hook was running: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the public cancellation did not return")
	}
	f.releaseHook()
	f.waitForHookFile(t, f.hookFinished)
	body, err := os.ReadFile(f.hookStarted)
	if err != nil {
		t.Fatal(err)
	}
	var input hook.Input
	if err := json.Unmarshal(body, &input); err != nil ||
		input.Event != hook.Stop || input.Reason != string(protocol.OutcomeCanceled) {
		t.Fatalf("the installed stop hook did not receive the committed outcome: %+v, %v", input, err)
	}
}

type heldRunHook struct {
	runtime      *poMCPRuntime
	ctx          context.Context
	started      *protocol.StartRunResponse
	events       <-chan runEventCollection
	watcher      *fsnotify.Watcher
	hookStarted  string
	hookFinished string
	releaseHook  func()
	releaseModel func()
}

func startRunWithHeldHook(t *testing.T, event hook.Event) heldRunHook {
	t.Helper()
	cfg := poMCPConfig(t)
	model := newLifecycleModel()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	const (
		startedMarker  = "hook-input.json"
		finishedMarker = "hook-finished"
		releasePipe    = "hook-release"
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
	t.Cleanup(func() { _ = pipe.Close() })
	command := `cat > ` + startedMarker +
		`; IFS= read -r release < ` + releasePipe +
		`; printf 'finished' > ` + finishedMarker + `; printf '%s' '{}'`
	type commandRule struct {
		Event   hook.Event `json:"event"`
		Command string     `json:"command"`
	}
	rules, err := json.Marshal(struct {
		Hooks []commandRule `json:"hooks"`
	}{Hooks: []commandRule{{Event: event, Command: command}}})
	if err != nil {
		t.Fatal(err)
	}
	writeHookSource(t, cfg.userHome, string(rules))
	runtime := poMCPOpen(t, cfg)
	releaseModel := sync.OnceFunc(func() { close(model.releaseFirstCall) })
	releaseHook := sync.OnceFunc(func() {
		if _, err := io.WriteString(pipe, "continue\n"); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(releaseModel)
	t.Cleanup(releaseHook)
	activateHookProvider(t, runtime.api)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := watcher.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := watcher.Add(workspace); err != nil {
		t.Fatal(err)
	}
	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "observe Run lifecycle hook",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, stream, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Start the Run."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectRunEvents(stream)
	waitForSignal(t, model.firstCallStarted, "the first model call")
	return heldRunHook{
		runtime: runtime, ctx: ctx, started: started, events: events, watcher: watcher,
		hookStarted: filepath.Join(workspace, startedMarker), hookFinished: filepath.Join(workspace, finishedMarker),
		releaseHook: releaseHook, releaseModel: releaseModel,
	}
}

func (f heldRunHook) waitForHookFile(t *testing.T, marker string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	for {
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				t.Fatal(err)
			}
			return
		}
		select {
		case <-f.watcher.Events:
		case err := <-f.watcher.Errors:
			t.Fatalf("observe the installed hook: %v", err)
		case <-ctx.Done():
			t.Fatalf("the installed hook did not create %s", filepath.Base(marker))
		}
	}
}
