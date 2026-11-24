package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"

	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func connectHooks(t *testing.T, path string) (sqlite.Database, *sdk.ClientSession, func()) {
	t.Helper()
	db, err := sqlite.OpenSchema(t.Context(), path, []string{pluginstorage.CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(db)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "hook-test", Version: "1"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), connection.Close(), db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	return db, session, release
}

func hookContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := pluginmcp.WithInvocation(t.Context(), framecall.Invocation{ID: "hook-test", Origin: extension.Automatic, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: "hooks"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func hookMethod[P, R any](t *testing.T, session *sdk.ClientSession, method *pluginmcp.Method[P, R]) *pluginmcp.BoundMethod[P, R] {
	t.Helper()
	contract, err := hookmcp.Contract()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := pluginmcp.NewCapabilityBinding(session, "hook-provider", contract)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := method.Bind(capability)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func TestOriginalHookCommandReceiptSurvivesRestartWithoutRepeatingItsEffect(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(t.TempDir(), "hooks.db")
	db, session, release := connectHooks(t, path)
	method, err := hookmcp.EvaluateMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Evaluation{Rules: []hook.Hook{{Event: hook.PreToolUse, Scope: hook.ScopeGlobal, Source: cwd, Command: `printf x >> effect; printf '%s' '{"decision":"deny","reason":"policy"}'`, TimeoutMillis: 1000}}, Input: hook.Input{Event: hook.PreToolUse, CWD: cwd}}
	first, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request)
	if err != nil || first.Decision.Verdict != hook.CommandDeny {
		t.Fatalf("original=%+v, %v", first, err)
	}
	if err := db.RunInTx(t.Context(), func(ctx context.Context) error {
		_, err := db.Executor(ctx).ExecContext(ctx, "UPDATE commands SET reply=NULL WHERE id=?", "original")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	release()
	_, session, _ = connectHooks(t, path)
	replayed, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request)
	if err != nil || replayed.Decision.Reason != "policy" {
		t.Fatalf("original receipt=%+v, %v", replayed, err)
	}
	request.Rules[0].Command = `printf y >> effect`
	if _, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request); err == nil {
		t.Fatal("original command identity accepted changed input")
	}
	bytes, err := os.ReadFile(filepath.Join(cwd, "effect"))
	if err != nil || string(bytes) != "x" {
		t.Fatalf("original effect=%q, %v", bytes, err)
	}
}

func TestDiscoverySkipsUntrustedProjectConfigurationUnderItsCapturedRoot(t *testing.T) {
	cwd, home := t.TempDir(), t.TempDir()
	writeHooks(t, home, `{"hooks":[{"event":"SessionStart","inject":"global"}]}`)
	writeHooks(t, cwd, `{ broken project rules`)
	_, session, _ := connectHooks(t, filepath.Join(t.TempDir(), "hooks.db"))
	method, err := hookmcp.DiscoverMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Discovery{CWD: cwd, RepositoryRoot: cwd, Home: home}
	result, err := hookMethod(t, session, method).Call(hookContext(t), request)
	if err != nil || len(result) != 1 || result[0].Inject != "global" || result[0].Scope != hook.ScopeGlobal {
		t.Fatalf("untrusted discovery=%+v, %v", result, err)
	}
	request.IncludeRepository = true
	if _, err := hookMethod(t, session, method).Call(hookContext(t), request); err == nil {
		t.Fatal("inspection hid malformed project rules")
	}
}
