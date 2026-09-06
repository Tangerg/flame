package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

type failingHookTrust struct{ err error }

func (f failingHookTrust) IsTrusted(context.Context, string) (bool, error) {
	return false, f.err
}

func TestNewHookResolverPreservesTrustStoreFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	wantErr := errors.New("trust store unavailable")
	resolver, err := NewHookResolver(t.TempDir(), failingHookTrust{err: wantErr})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.For(context.Background(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("For error = %v, want %v", err, wantErr)
	}
}

func TestNewHookResolverRequiresTrustStore(t *testing.T) {
	for _, trust := range []HookTrust{nil, (*failingHookTrust)(nil)} {
		if resolver, err := NewHookResolver(t.TempDir(), trust); err == nil || resolver != nil {
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"hooks":[{"event":"Stop","command":"exit 7"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewHookResolver(home, failingHookTrust{})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	decision := bound.Run(t.Context(), domainhooks.Input{Event: domainhooks.Stop, SessionID: "session:one", CWD: home})
	if decision.Block || decision.Ask {
		t.Fatalf("broken observe-only command changed the lifecycle decision: %+v", decision)
	}
	if output := diagnostics.String(); !strings.Contains(output, "exit status 7") || !strings.Contains(output, configPath) {
		t.Fatalf("command failure lost its source or cause: %s", output)
	}
}
