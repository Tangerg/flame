package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
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
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "hook-test", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "hooks"}})
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
	db, session, close := connectHooks(t, path)
	method, err := hookmcp.EvaluateMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Evaluation{Rules: []hook.Hook{{Event: hook.PreToolUse, Scope: hook.ScopeGlobal, Source: cwd, Command: `printf x >> effect; printf '%s' '{"decision":"deny","reason":"policy"}'`, TimeoutMillis: 1000}}, Input: hook.Input{Event: hook.PreToolUse, CWD: cwd}}
	first, err := hookMethod(t, session, method).CallCommand(hookContext(t), "original", request)
	if err != nil || first.Decision.Verdict != hook.CommandDeny {
		t.Fatalf("original=%+v, %v", first, err)
	}
	// Model a crash after the shell receipt commits but before the enclosing event reply.
	if _, err := db.ExecContext(t.Context(), "UPDATE commands SET reply=NULL WHERE id=?", "original"); err != nil {
		t.Fatal(err)
	}

	close()
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

func TestPendingHookCommandCannotDispatchAndTimeoutKeepsItsKnownOutcome(t *testing.T) {
	cwd := t.TempDir()
	db, session, _ := connectHooks(t, filepath.Join(t.TempDir(), "hooks.db"))
	method, err := hookmcp.EvaluateMethod()
	if err != nil {
		t.Fatal(err)
	}
	request := hookprovider.Evaluation{Rules: []hook.Hook{{Event: hook.PreToolUse, Scope: hook.ScopeGlobal, Source: cwd, Command: `printf x > effect`, TimeoutMillis: 1000}}, Input: hook.Input{Event: hook.PreToolUse, CWD: cwd}}
	input, err := framevalue.EncodeJSON(request)
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
	request.Rules[0].Command, request.Rules[0].TimeoutMillis = "sleep 5", 40
	for range 2 {
		result, err := hookMethod(t, session, method).CallCommand(hookContext(t), "timeout", request)
		if err != nil || len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Message, "timed out") {
			t.Fatalf("timeout outcome=%+v, %v", result, err)
		}
	}
}

func TestDeclarativeOnlyEventPreservesRequiredPolicyWithoutRunningACommand(t *testing.T) {
	cwd := t.TempDir()
	_, session, _ := connectHooks(t, filepath.Join(t.TempDir(), "hooks.db"))
	method, err := hookmcp.EvaluateMethod()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		policy      hook.FailurePolicy
		matcher     string
		verdict     hook.CommandVerdict
		diagnostics int
	}{
		{name: "required", policy: hook.FailureRequired, verdict: hook.CommandDeny, diagnostics: 1},
		{name: "advisory", verdict: hook.CommandAllow, diagnostics: 1},
		{name: "unmatched", policy: hook.FailureRequired, matcher: "write", verdict: hook.CommandAllow},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := hookprovider.Evaluation{
				Rules: []hook.Hook{
					{Event: hook.PreToolUse, Command: "printf forbidden > effect", FailurePolicy: test.policy, Matcher: test.matcher, Scope: hook.ScopeGlobal, Source: cwd},
					{Event: hook.PreToolUse, Inject: "retained declarative context", Scope: hook.ScopeGlobal, Source: cwd},
				},
				Input:           hook.Input{Event: hook.PreToolUse, CWD: cwd, Tool: &hook.ToolInput{Name: "shell"}},
				DeclarativeOnly: true,
			}
			for range 2 {
				advice, err := hookMethod(t, session, method).CallCommand(hookContext(t), pluginmcp.CommandID(test.name), request)
				if err != nil || advice.Decision.Verdict != test.verdict || advice.Decision.InjectContext != "retained declarative context" || len(advice.Diagnostics) != test.diagnostics {
					t.Fatalf("declarative-only original receipt: %+v, %v", advice, err)
				}
			}
			if _, err := os.Stat(filepath.Join(cwd, "effect")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unprojectable command ran: %v", err)
			}
		})
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
