package bootstrap

import (
	"context"
	"errors"
	"testing"
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
