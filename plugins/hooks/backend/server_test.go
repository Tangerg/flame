package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func connectHooks(t *testing.T, path string) (*sql.DB, *sdk.ClientSession, func()) {
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
	if err := qimumcp.RegisterClient(client); err != nil {
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
	close := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), connection.Close(), db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(close)
	return db, session, close
}

func hookContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "hook-test", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "hooks"}})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func hookMethod[P, R any](t *testing.T, session *sdk.ClientSession, method *qimumcp.Method[P, R]) *qimumcp.BoundMethod[P, R] {
	t.Helper()
	contract, err := hookprovider.Contract()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := qimumcp.NewCapabilityBinding(session, "hook-provider", contract)
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
	_, session, close := connectHooks(t, path)
	method, err := hookprovider.ExecuteMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Command{Command: `printf x >> effect; printf '%s' '{"decision":"deny","reason":"policy"}'`, CWD: cwd, Input: hook.Input{Event: hook.PreToolUse, CWD: cwd}, TimeoutMillis: 1000}
	first, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request)
	if err != nil || first.Decision == nil || first.Decision.Verdict != hook.CommandDeny {
		t.Fatalf("original=%+v, %v", first, err)
	}
	close()
	_, session, _ = connectHooks(t, path)
	replayed, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request)
	if err != nil || replayed.Decision == nil || replayed.Decision.Reason != "policy" {
		t.Fatalf("original receipt=%+v, %v", replayed, err)
	}
	request.Command = `printf y >> effect`
	if _, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request); err == nil {
		t.Fatal("original command identity accepted changed input")
	}
	bytes, err := os.ReadFile(filepath.Join(cwd, "effect"))
	if err != nil || string(bytes) != "x" {
		t.Fatalf("original effect=%q, %v", bytes, err)
	}
}

func TestPendingHookCommandCannotDispatchAndTimeoutKeepsItsKnownOutcome(t *testing.T) {
	cwd := t.TempDir()
	db, session, _ := connectHooks(t, filepath.Join(t.TempDir(), "hooks.db"))
	method, err := hookprovider.ExecuteMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Command{Command: `printf x > effect`, CWD: cwd, Input: hook.Input{Event: hook.PreToolUse, CWD: cwd}, TimeoutMillis: 1000}
	input, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pluginstorage.AdmitCommand(t.Context(), db, method.Name(), "pending", input, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := hookMethod(t, session, method).CallCommand(hookContext(t), "pending", request); err == nil {
		t.Fatal("unconfirmed command was dispatched again")
	}
	if _, err := os.Stat(filepath.Join(cwd, "effect")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending command produced an effect: %v", err)
	}
	request.Command, request.TimeoutMillis = "sleep 5", 40
	for range 2 {
		result, err := hookMethod(t, session, method).CallCommand(hookContext(t), "timeout", request)
		if err != nil || result.Failure == nil || result.Failure.Kind != hookprovider.TimedOut {
			t.Fatalf("timeout outcome=%+v, %v", result, err)
		}
	}
}

func TestDiscoverySkipsUntrustedProjectConfigurationUnderItsCapturedRoot(t *testing.T) {
	cwd, home := t.TempDir(), t.TempDir()
	writeHooks(t, home, `{"hooks":[{"event":"SessionStart","inject":"global"}]}`)
	writeHooks(t, cwd, `{ broken project rules`)
	_, session, _ := connectHooks(t, filepath.Join(t.TempDir(), "hooks.db"))
	method, err := hookprovider.DiscoverMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Discovery{CWD: cwd, ProjectRoot: cwd, Home: home}
	result, err := hookMethod(t, session, method).Call(hookContext(t), request)
	if err != nil || len(result) != 1 || result[0].Inject != "global" || result[0].Scope != hook.ScopeGlobal {
		t.Fatalf("untrusted discovery=%+v, %v", result, err)
	}
	request.IncludeProject = true
	if _, err := hookMethod(t, session, method).Call(hookContext(t), request); err == nil {
		t.Fatal("inspection hid malformed project rules")
	}
}
