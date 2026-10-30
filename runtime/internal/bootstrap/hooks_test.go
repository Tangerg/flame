package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

type failingHookTrust struct{ err error }

func (f failingHookTrust) IsTrusted(context.Context, string) (bool, error) {
	return false, f.err
}

func TestNewHookResolverPreservesTrustStoreFailure(t *testing.T) {
	wantErr := errors.New("trust store unavailable")
	resolver, err := newHookResolver(t.TempDir(), failingHookTrust{err: wantErr}, hookSourceFixture{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.For(context.Background(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("For error = %v, want %v", err, wantErr)
	}
}

func TestNewHookResolverRequiresTrustStore(t *testing.T) {
	for _, trust := range []hookTrust{nil, (*failingHookTrust)(nil)} {
		if resolver, err := newHookResolver(t.TempDir(), trust, hookSourceFixture{}); err == nil || resolver != nil {
			t.Fatalf("NewHookResolver = (%v, %v), want missing trust rejected", resolver, err)
		}
	}
}

func TestHookCommandFailureIsReportedWithoutAnActiveSpan(t *testing.T) {
	var diagnostics bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	home := t.TempDir()
	configPath := filepath.Join(home, ".flame", "hooks.json")
	source := hookSourceFixture{rules: []hookdata.Hook{{Event: hookdata.Stop, Command: "exit 7", Scope: hookdata.ScopeGlobal, Source: configPath}}, failure: errors.New("exit status 7")}
	resolver, err := newHookResolver(home, failingHookTrust{}, source)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bound.Close(); err != nil {
			t.Error(err)
		}
	}()
	decision := bound.Run(t.Context(), hookdata.Input{Event: hookdata.Stop, SessionID: "session:one", CWD: home})
	if decision.Verdict != domainhooks.VerdictAllow {
		t.Fatalf("broken observe-only command changed the lifecycle decision: %+v", decision)
	}
	if output := diagnostics.String(); !strings.Contains(output, "exit status 7") || !strings.Contains(output, configPath) {
		t.Fatalf("command failure lost its source or cause: %s", output)
	}
}

type hookSourceFixture struct {
	rules   []hookdata.Hook
	failure error
}

func (s hookSourceFixture) AcquireHooks(context.Context) (adapterhooks.Use, error) { return s, nil }
func (s hookSourceFixture) Discover(context.Context, hookprovider.Discovery) ([]hookdata.Hook, error) {
	return append([]hookdata.Hook(nil), s.rules...), nil
}
func (s hookSourceFixture) RunHookCommand(context.Context, apphooks.CommandRequest) apphooks.CommandResult {
	return apphooks.CommandResult{Err: s.failure, ExitCode: 7}
}
func (hookSourceFixture) Close() error { return nil }
